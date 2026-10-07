// Package api implements barnd's HTTP API (see api/openapi.yaml) and WebSocket endpoint.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/auth"
	"github.com/etchebarne/barn/internal/bus"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/runtime"
	"github.com/etchebarne/barn/internal/settings"
	"github.com/etchebarne/barn/internal/store"
)

const (
	sessionCookie = "barn_session"
	sessionTTL    = 30 * 24 * time.Hour
	csrfHeader    = "X-Barn-CSRF"
)

type Options struct {
	SecureCookies  bool
	AllowedOrigins []string
	// Web is the built web app to serve at /. Nil serves the API only.
	Web fs.FS
}

type Server struct {
	store    *store.Store
	bus      *bus.Bus
	runtime  *runtime.Manager
	llm      *model.Client
	settings *settings.Settings
	opts     Options

	loginLimiter *auth.Limiter
}

var _ gen.ServerInterface = (*Server)(nil)

func New(s *store.Store, b *bus.Bus, rt *runtime.Manager, llm *model.Client, set *settings.Settings, opts Options) *Server {
	return &Server{
		store: s, bus: b, runtime: rt, llm: llm, settings: set, opts: opts,
		loginLimiter: auth.NewLimiter(12*time.Second, 5),
	}
}

// Handler returns the root HTTP handler: API, WebSocket, and (optionally) the web app.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	gen.HandlerWithOptions(s, gen.StdHTTPServerOptions{
		BaseURL:    "/api",
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
	})
	mux.HandleFunc("GET /api/ws", s.serveWS)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if s.opts.Web != nil {
		mux.Handle("/", spaHandler(s.opts.Web))
	}
	return recoverer(securityHeaders(s.csrf(s.authenticate(mux))))
}

// publicRoutes don't require a session.
var publicRoutes = map[string]bool{
	"GET /api/auth/status": true,
	"POST /api/auth/setup": true,
	"POST /api/auth/login": true,
}

type ctxKey int

const userKey ctxKey = 0

func userFrom(ctx context.Context) store.User {
	u, _ := ctx.Value(userKey).(store.User)
	return u
}

// authenticate resolves the session cookie and rejects unauthenticated API requests.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := s.sessionUser(r); ok {
			r = r.WithContext(context.WithValue(r.Context(), userKey, u))
		} else if isAPI(r) && !publicRoutes[r.Method+" "+r.URL.Path] {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sessionUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, err := s.store.SessionUser(r.Context(), auth.HashToken(c.Value), sessionTTL)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("session lookup", "err", err)
		}
		return store.User{}, false
	}
	return u, true
}

// csrf requires a custom header on mutating API requests. Browsers can't send custom headers
// cross-origin without a CORS preflight, which barnd never approves.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if isAPI(r) && r.Header.Get(csrfHeader) != "1" {
				writeError(w, http.StatusForbidden, "missing "+csrfHeader+" header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		if isAPI(r) {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic in handler", "panic", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func isAPI(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, gen.Error{Message: msg})
}

func internalError(w http.ResponseWriter, err error) {
	slog.Error("internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// decode reads a JSON request body (max 1 MiB) into v.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

package api

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/auth"
	"github.com/etchebarne/openbot/internal/store"
)

func (s *Server) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	status := gen.AuthStatus{SetupRequired: n == 0}
	if u, ok := r.Context().Value(userKey).(store.User); ok {
		status.User = &gen.User{Id: u.ID, Username: u.Username}
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) SetupAccount(w http.ResponseWriter, r *http.Request) {
	var req gen.Credentials
	if !decode(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if msg := validateCredentials(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(w, err)
		return
	}
	u, err := s.store.CreateFirstUser(r.Context(), req.Username, hash)
	if errors.Is(err, store.ErrUserExists) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gen.User{Id: u.ID, Username: u.Username})
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again in a minute")
		return
	}
	var req gen.Credentials
	if !decode(w, r, &req) {
		return
	}
	u, err := s.store.UserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if errors.Is(err, store.ErrNotFound) {
		auth.VerifyDummy(req.Password)
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	ok, err := auth.VerifyPassword(req.Password, u.PasswordHash)
	if err != nil {
		internalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	if err := s.startSession(w, r, u); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gen.User{Id: u.ID, Username: u.Username})
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			internalError(w, err)
			return
		}
	}
	http.SetCookie(w, s.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.User) error {
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		return err
	}
	if err := s.store.CreateSession(r.Context(), hash, u.ID, sessionTTL); err != nil {
		return err
	}
	http.SetCookie(w, s.cookie(token, int(sessionTTL.Seconds())))
	return nil
}

func (s *Server) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.opts.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	}
}

func validateCredentials(c gen.Credentials) string {
	switch {
	case c.Username == "" || utf8.RuneCountInString(c.Username) > 64:
		return "username must be 1–64 characters"
	case utf8.RuneCountInString(c.Password) < 12:
		return "password must be at least 12 characters"
	case len(c.Password) > 256:
		return "password is too long"
	}
	return ""
}

// clientIP is the TCP peer address. openbotd doesn't trust proxy headers.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

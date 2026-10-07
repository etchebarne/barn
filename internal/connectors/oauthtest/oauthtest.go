// Package oauthtest is a fake MCP server with its own OAuth authorization server, for tests.
package oauthtest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Server is an MCP server with its own authorization server, following the MCP
// authorization spec closely enough to catch mistakes: PKCE is checked, codes are single
// use, refresh tokens rotate, and plain-http redirects can be refused like Linear does.
type Server struct {
	*httptest.Server
	RejectHTTP bool

	mu        sync.Mutex
	clients   map[string]string            // client id -> redirect uri
	codes     map[string]map[string]string // code -> challenge, client, redirect, resource
	access    string
	refresh   string
	Refreshes int
	n         int
}

func New(t *testing.T, rejectHTTP bool) *Server {
	f := &Server{RejectHTTP: rejectHTTP, clients: map[string]string{}, codes: map[string]map[string]string{}}
	mux := http.NewServeMux()
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"resource": f.URL + "/mcp", "authorization_servers": []string{f.URL}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"issuer": f.URL, "authorization_endpoint": f.URL + "/authorize",
			"token_endpoint": f.URL + "/token", "registration_endpoint": f.URL + "/register",
			"code_challenge_methods_supported": []string{"S256"}})
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RedirectURIs []string `json:"redirect_uris"`
			AuthMethod   string   `json:"token_endpoint_auth_method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		u, _ := url.Parse(req.RedirectURIs[0])
		if f.RejectHTTP && u.Scheme == "http" && !isLoopback(u.Hostname()) {
			writeJSON(w, 400, map[string]string{"error": "invalid_redirect_uri", "error_description": "plaintext HTTP is allowed only for loopback"})
			return
		}
		f.mu.Lock()
		f.n++
		id := fmt.Sprintf("client-%d", f.n)
		f.clients[id] = req.RedirectURIs[0]
		f.mu.Unlock()
		writeJSON(w, 201, map[string]any{"client_id": id, "redirect_uris": req.RedirectURIs})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			c, ok := f.codes[r.Form.Get("code")]
			delete(f.codes, r.Form.Get("code"))
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || c["client"] != r.Form.Get("client_id") || c["redirect"] != r.Form.Get("redirect_uri") ||
				c["challenge"] != base64.RawURLEncoding.EncodeToString(sum[:]) || r.Form.Get("resource") != f.URL+"/mcp" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
		case "refresh_token":
			if f.refresh == "" || r.Form.Get("refresh_token") != f.refresh {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.Refreshes++
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
			return
		}
		f.n++
		f.access, f.refresh = fmt.Sprintf("access-%d", f.n), fmt.Sprintf("refresh-%d", f.n)
		writeJSON(w, 200, map[string]any{"access_token": f.access, "refresh_token": f.refresh, "token_type": "Bearer", "expires_in": 3600})
	})
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		ok := f.access != "" && r.Header.Get("Authorization") == "Bearer "+f.access
		f.mu.Unlock()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+f.URL+`/.well-known/oauth-protected-resource/mcp", scope="read write"`)
			w.WriteHeader(401)
			return
		}
		var msg struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&msg)
		if msg.ID == nil {
			w.WriteHeader(202)
			return
		}
		var result any = map[string]any{}
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "whoami", "annotations": map[string]any{"readOnlyHint": true}}}}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "you are martin"}}}
		}
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
	})
	return f
}

// approve plays the user clicking Allow: it checks the authorize request and returns the URL
// the browser lands on.
func (f *Server) Approve(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, f.URL+"/authorize?") {
		t.Fatalf("authorize URL = %s", authURL)
	}
	q := u.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	redirect := f.clients[q.Get("client_id")]
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("redirect_uri") != redirect || q.Get("resource") != f.URL+"/mcp" || q.Get("scope") != "read write" || q.Get("state") == "" {
		t.Fatalf("bad authorize request: %s", authURL)
	}
	f.n++
	code := fmt.Sprintf("code-%d", f.n)
	f.codes[code] = map[string]string{"challenge": q.Get("code_challenge"), "client": q.Get("client_id"), "redirect": redirect}
	return redirect + "?code=" + code + "&state=" + url.QueryEscape(q.Get("state"))
}

func (f *Server) ExpireAccess() {
	f.mu.Lock()
	f.access = "rotated-elsewhere"
	f.mu.Unlock()
}

func (f *Server) Revoke() {
	f.mu.Lock()
	f.access, f.refresh = "", ""
	f.mu.Unlock()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/etchebarne/barn/internal/bus"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/runtime"
	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/settings"
	"github.com/etchebarne/barn/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newTestServerWithProvider(t, "http://127.0.0.1:0")
	return ts
}

func newTestServerWithProvider(t *testing.T, providerURL string) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	box, err := secrets.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	set := settings.New(st, box)
	b := bus.New()
	llm := model.New(providerURL, "barn/test", set.APIKey)
	rt := runtime.New(st, b, llm)
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(New(st, b, rt, llm, set, Options{}).Handler())
	t.Cleanup(func() {
		ts.Close()
		cancel()
		rt.Wait()
		st.Close()
	})
	return ts, st
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, ts *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path, body string, csrf bool) (*http.Response, map[string]any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		req.Header.Set(csrfHeader, "1")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAuthFlow(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)

	_, status := c.do("GET", "/api/auth/status", "", false)
	if status["setupRequired"] != true || status["user"] != nil {
		t.Fatalf("unexpected initial status: %v", status)
	}

	if resp, _ := c.do("GET", "/api/chats", "", false); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 before login, got %d", resp.StatusCode)
	}

	creds := `{"username":"martin","password":"a long enough password"}`
	if resp, _ := c.do("POST", "/api/auth/setup", creds, false); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF header, got %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/auth/setup", `{"username":"martin","password":"short"}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", resp.StatusCode)
	}
	if resp, body := c.do("POST", "/api/auth/setup", creds, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup failed: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("POST", "/api/auth/setup", creds, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for second setup, got %d", resp.StatusCode)
	}

	_, status = c.do("GET", "/api/auth/status", "", false)
	if user, _ := status["user"].(map[string]any); user["username"] != "martin" {
		t.Fatalf("expected signed-in user, got %v", status)
	}
	_, onboarding := c.do("GET", "/api/onboarding", "", false)
	if onboarding["completed"] != false || onboarding["providerConfigured"] != false {
		t.Fatalf("unexpected onboarding state: %v", onboarding)
	}

	if resp, _ := c.do("POST", "/api/auth/logout", "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout failed: %d", resp.StatusCode)
	}
	if resp, _ := c.do("GET", "/api/chats", "", false); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", resp.StatusCode)
	}

	other := newClient(t, ts)
	if resp, _ := other.do("POST", "/api/auth/login", `{"username":"martin","password":"wrong password!!"}`, true); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}
	if resp, _ := other.do("POST", "/api/auth/login", creds, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d", resp.StatusCode)
	}
	if resp, _ := other.do("GET", "/api/chats", "", false); resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after login, got %d", resp.StatusCode)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	c.do("POST", "/api/auth/setup", `{"username":"martin","password":"a long enough password"}`, true)
	var last int
	for range 7 {
		resp, _ := c.do("POST", "/api/auth/login", `{"username":"martin","password":"wrong password!!"}`, true)
		last = resp.StatusCode
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
}

func TestUpdateAgentModel(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model-a"}, {"id": "model-b"}}})
	}))
	defer provider.Close()
	ts, st := newTestServerWithProvider(t, provider.URL)
	c := newClient(t, ts)
	c.do("POST", "/api/auth/setup", `{"username":"martin","password":"a long enough password"}`, true)

	agent, _, err := st.CreateAgentWithDM(context.Background(), store.Agent{
		Name: "barn", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, body := c.do("PATCH", "/api/agents/"+agent.ID, `{"model":"model-b"}`, true)
	if resp.StatusCode != http.StatusOK || body["model"] != "model-b" {
		t.Fatalf("update failed: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("PATCH", "/api/agents/"+agent.ID, `{"model":"nope"}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown model, got %d", resp.StatusCode)
	}
	if resp, _ := c.do("PATCH", "/api/agents/missing", `{"model":"model-a"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown agent, got %d", resp.StatusCode)
	}
	got, _ := st.GetAgent(context.Background(), agent.ID)
	if got.Model != "model-b" {
		t.Fatalf("stored model = %q", got.Model)
	}
}

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

// fakeProvider serves a model list and chat completions. "blocked" behaves like a model whose
// provider trains on request data.
func fakeProvider(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{
				{"id": "model-a"}, {"id": "model-b"}, {"id": "blocked"},
			}})
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "blocked" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"Upstream request failed: This Go model trains on request data."}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": ""}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupWithKey(t *testing.T) (*client, *store.Store) {
	t.Helper()
	ts, st := newTestServerWithProvider(t, fakeProvider(t).URL)
	c := newClient(t, ts)
	c.do("POST", "/api/auth/setup", `{"username":"martin","password":"a long enough password"}`, true)
	if resp, body := c.do("PUT", "/api/settings/provider", `{"apiKey":"test-key"}`, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("save key: %d %v", resp.StatusCode, body)
	}
	return c, st
}

func TestUpdateAgentModel(t *testing.T) {
	c, st := setupWithKey(t)
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
	resp, body = c.do("PATCH", "/api/agents/"+agent.ID, `{"model":"blocked"}`, true)
	if msg, _ := body["message"].(string); resp.StatusCode != http.StatusBadRequest || !strings.Contains(msg, "Privacy settings") {
		t.Fatalf("expected a privacy error for a blocked model, got %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("PATCH", "/api/agents/missing", `{"model":"model-a"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown agent, got %d", resp.StatusCode)
	}
	got, _ := st.GetAgent(context.Background(), agent.ID)
	if got.Model != "model-b" {
		t.Fatalf("stored model = %q", got.Model)
	}
}

func TestOnboardingRejectsBlockedModel(t *testing.T) {
	c, st := setupWithKey(t)
	resp, body := c.do("POST", "/api/onboarding/complete", `{"model":"blocked"}`, true)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %v", resp.StatusCode, body)
	}
	if n, _ := st.CountAgents(context.Background()); n != 0 {
		t.Fatalf("no agent should be created for a blocked model, got %d", n)
	}
	if resp, body := c.do("POST", "/api/onboarding/complete", `{"model":"model-a"}`, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("onboarding with an allowed model failed: %d %v", resp.StatusCode, body)
	}
}

func TestRetryAgent(t *testing.T) {
	c, st := setupWithKey(t)
	agent, _, err := st.CreateAgentWithDM(context.Background(), store.Agent{
		Name: "barn", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp, _ := c.do("POST", "/api/agents/"+agent.ID+"/retry", "", true); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/agents/missing/retry", "", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestAnswerPrompt(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	agent, chatID, err := st.CreateAgentWithDM(ctx, store.Agent{
		Name: "barn", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	ask := func(kind string, allowOther bool) string {
		p := store.Prompt{Kind: kind, Question: "Q?", AllowOther: allowOther}
		if kind != "text" {
			p.Options = []store.PromptOption{{Label: "A"}, {Label: "B"}}
		}
		msg, err := st.InsertPrompt(ctx, chatID, agent.ID, p)
		if err != nil {
			t.Fatal(err)
		}
		return msg.ID
	}

	single := ask("single", false)
	for _, body := range []string{`{}`, `{"selected":[0,1]}`, `{"selected":[5]}`, `{"text":"other"}`} {
		if resp, _ := c.do("POST", "/api/messages/"+single+"/answer", body, true); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("single %s: expected 400, got %d", body, resp.StatusCode)
		}
	}
	resp, body := c.do("POST", "/api/messages/"+single+"/answer", `{"selected":[1]}`, true)
	prompt, _ := body["prompt"].(map[string]any)
	if resp.StatusCode != http.StatusOK || prompt["status"] != "answered" {
		t.Fatalf("answer failed: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("POST", "/api/messages/"+single+"/answer", `{"selected":[0]}`, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for a second answer, got %d", resp.StatusCode)
	}

	multi := ask("multi", true)
	if resp, body := c.do("POST", "/api/messages/"+multi+"/answer", `{"selected":[0,1],"text":"C"}`, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("multi with other: %d %v", resp.StatusCode, body)
	}

	text := ask("text", false)
	if resp, _ := c.do("POST", "/api/messages/"+text+"/answer", `{"selected":[0]}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("text with selection: expected 400, got %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/messages/"+text+"/answer", `{"text":"hello"}`, true); resp.StatusCode != http.StatusOK {
		t.Fatalf("text answer: expected 200, got %d", resp.StatusCode)
	}

	dismissed := ask("single", false)
	resp, body = c.do("POST", "/api/messages/"+dismissed+"/dismiss", "", true)
	prompt, _ = body["prompt"].(map[string]any)
	if resp.StatusCode != http.StatusOK || prompt["status"] != "dismissed" {
		t.Fatalf("dismiss failed: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("POST", "/api/messages/"+dismissed+"/answer", `{"selected":[0]}`, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 answering a dismissed prompt, got %d", resp.StatusCode)
	}

	plain, _ := st.InsertMessage(ctx, chatID, "user", nil, "hi")
	if resp, _ := c.do("POST", "/api/messages/"+plain.ID+"/answer", `{"selected":[0]}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for a non-prompt message, got %d", resp.StatusCode)
	}
}

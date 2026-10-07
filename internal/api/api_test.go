package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	pngenc "image/png"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/attachments"
	"github.com/etchebarne/openbot/internal/bus"
	"github.com/etchebarne/openbot/internal/connectors"
	"github.com/etchebarne/openbot/internal/connectors/oauthtest"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/settings"
	"github.com/etchebarne/openbot/internal/store"
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
	llm := model.New(providerURL, "openbot/test", set.APIKey)
	rt := runtime.New(st, b, llm)
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	srv := New(st, b, rt, llm, set, nil, Options{PublicURL: "https://openbot.example.com"})
	conns := connectors.NewManager(st, box)
	rt.Connectors, srv.Connectors = conns, conns
	files := &attachments.Files{Dir: filepath.Join(dir, "shared", "attachments"), Store: st}
	rt.Files, srv.Files = files, files
	ts := httptest.NewServer(srv.Handler())
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

func (c *client) doList(method, path string) (*http.Response, []any) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []any
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
		Name: "openbot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
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

	// Personality: set (trimmed), shown, cleared.
	resp, body = c.do("PATCH", "/api/agents/"+agent.ID, `{"personality":" Calm and dry. "}`, true)
	if resp.StatusCode != http.StatusOK || body["personality"] != "Calm and dry." {
		t.Fatalf("personality: %d %v", resp.StatusCode, body)
	}
	if _, body = c.do("PATCH", "/api/agents/"+agent.ID, `{"personality":""}`, true); body["personality"] != "" {
		t.Fatalf("clearing personality: %v", body)
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
		Name: "openbot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
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
		Name: "openbot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask",
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

func TestUserReactionsAndDeleting(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	admin, chatID, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "openbot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask", IsAdmin: true})
	other, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Notes", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	msg, _ := st.InsertMessage(ctx, chatID, "agent", &admin.ID, "hello")

	resp, body := c.do("POST", "/api/messages/"+msg.ID+"/reactions", `{"emoji":"🎉"}`, true)
	reactions, _ := body["reactions"].([]any)
	if resp.StatusCode != http.StatusOK || len(reactions) != 1 {
		t.Fatalf("react: %d %v", resp.StatusCode, body)
	}
	pending, _ := st.PendingEvents(ctx, admin.ID)
	if len(pending) == 0 || pending[len(pending)-1].Kind != "reaction" {
		t.Fatalf("the agent should be told about the reaction, events %+v", pending)
	}
	resp, body = c.do("POST", "/api/messages/"+msg.ID+"/reactions", `{"emoji":"🎉"}`, true)
	if reactions, _ := body["reactions"].([]any); resp.StatusCode != http.StatusOK || len(reactions) != 0 {
		t.Fatalf("reacting again should remove it: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("POST", "/api/messages/"+msg.ID+"/reactions", `{"emoji":"lol"}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a non-emoji, got %d", resp.StatusCode)
	}

	if resp, _ := c.do("DELETE", "/api/agents/"+admin.ID, "", true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("the last admin can't be deleted, got %d", resp.StatusCode)
	}
	if resp, _ := c.do("DELETE", "/api/agents/"+other.ID, "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	_, list := c.doList("GET", "/api/chats")
	if len(list) != 1 {
		t.Fatalf("the deleted agent's DM should be gone, have %d chats", len(list))
	}

	resp, body = c.do("PATCH", "/api/agents/"+admin.ID, `{"name":"Openbot","trustMode":"trusted","instructions":"Be brief."}`, true)
	if resp.StatusCode != http.StatusOK || body["name"] != "Openbot" || body["trustMode"] != "trusted" || body["instructions"] != "Be brief." {
		t.Fatalf("update: %d %v", resp.StatusCode, body)
	}
}

func TestConnectorsAPI(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Write([]byte(`{"login":"martin"}`))
	}))
	defer gh.Close()
	c, st := setupWithKey(t)
	ctx := context.Background()
	agent, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})

	_, types := c.doList("GET", "/api/connectors/types")
	if len(types) < 6 {
		t.Fatalf("expected the connector types, got %d", len(types))
	}

	bad := `{"type":"github","name":"GitHub","credentials":{"token":"bad"},"config":{"base_url":"` + gh.URL + `"}}`
	if resp, body := c.do("POST", "/api/connectors", bad, true); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body["message"].(string), "Bad credentials") {
		t.Fatalf("bad credentials should be rejected readably: %d %v", resp.StatusCode, body)
	}
	good := `{"type":"github","name":"GitHub","credentials":{"token":"good","webhook_secret":"s"},"config":{"base_url":"` + gh.URL + `"},"agentIds":["` + agent.ID + `"]}`
	resp, body := c.do("POST", "/api/connectors", good, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %v", resp.StatusCode, body)
	}
	id := body["id"].(string)
	if body["webhookUrl"] != "https://openbot.example.com/hooks/"+id || !strings.Contains(fmt.Sprint(body["credentialsSet"]), "token") ||
		strings.Contains(fmt.Sprint(body), "good") {
		t.Fatalf("view leaks or misses fields: %v", body)
	}
	if ids, _ := body["agentIds"].([]any); len(ids) != 1 {
		t.Fatalf("grants = %v", body["agentIds"])
	}
	resp, body = c.do("PATCH", "/api/connectors/"+id, `{"name":"GitHub (work)","agentIds":[]}`, true)
	if resp.StatusCode != http.StatusOK || body["name"] != "GitHub (work)" || len(body["agentIds"].([]any)) != 0 {
		t.Fatalf("update: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("DELETE", "/api/connectors/"+id, "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if _, list := c.doList("GET", "/api/connectors"); len(list) != 0 {
		t.Fatalf("expected no connectors, got %v", list)
	}

	// Webhook accounts get a URL with their secret token; the hook route is public.
	resp, body = c.do("POST", "/api/connectors", `{"type":"webhook","name":"Pings","credentials":{}}`, true)
	if resp.StatusCode != http.StatusCreated || !strings.Contains(body["webhookUrl"].(string), "?token=") {
		t.Fatalf("webhook connector: %d %v", resp.StatusCode, body)
	}
	hookURL := strings.Replace(body["webhookUrl"].(string), "https://openbot.example.com", c.base, 1)
	anon := &http.Client{}
	r, err := anon.Post(hookURL, "application/json", strings.NewReader(`{"ok":true}`))
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("public webhook delivery: %v %v", err, r.StatusCode)
	}
	r.Body.Close()
}

func TestConnectPromptAPI(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Write([]byte(`{"login":"martin"}`))
	}))
	defer gh.Close()
	c, st := setupWithKey(t)
	ctx := context.Background()
	agent, dm, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	card := func() string {
		msg, err := st.InsertPrompt(ctx, dm, agent.ID, store.Prompt{
			Kind: "connect", Question: "Connect GitHub (GitHub)?", Options: []store.PromptOption{{Label: "Connect"}, {Label: "Decline"}},
			Connection: &store.PendingConnection{AgentID: agent.ID, Type: "github", Name: "GitHub",
				Config: map[string]string{"base_url": gh.URL}, AgentIDs: []string{agent.ID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return msg.ID
	}

	id := card()
	if resp, _ := c.do("POST", "/api/messages/"+id+"/answer", `{"selected":[0]}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("connecting must go through /connect, got %d", resp.StatusCode)
	}
	resp, body := c.do("POST", "/api/messages/"+id+"/connect", `{"credentials":{"token":"bad"}}`, true)
	if resp.StatusCode != http.StatusBadRequest || !strings.HasPrefix(body["message"].(string), "The service rejected the credentials") {
		t.Fatalf("bad token: %d %v", resp.StatusCode, body)
	}
	resp, body = c.do("POST", "/api/messages/"+id+"/connect", `{"credentials":{"token":"good"},"name":"Work GitHub"}`, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect: %d %v", resp.StatusCode, body)
	}
	p := body["prompt"].(map[string]any)
	conn := p["connection"].(map[string]any)
	if p["status"] != "answered" || conn["accountId"] == nil || conn["name"] != "Work GitHub" || strings.Contains(fmt.Sprint(body), "good") {
		t.Fatalf("answered card = %v", p)
	}
	if resp, _ := c.do("POST", "/api/messages/"+id+"/connect", `{"credentials":{"token":"good"}}`, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second connect: %d", resp.StatusCode)
	}
	_, list := c.doList("GET", "/api/connectors")
	if len(list) != 1 || fmt.Sprint(list[0].(map[string]any)["agentIds"]) != "["+agent.ID+"]" {
		t.Fatalf("connectors = %v", list)
	}

	id = card()
	if resp, body := c.do("POST", "/api/messages/"+id+"/answer", `{"selected":[1]}`, true); resp.StatusCode != http.StatusOK || body["prompt"].(map[string]any)["connection"].(map[string]any)["accountId"] != nil {
		t.Fatalf("decline: %d %v", resp.StatusCode, body)
	}
	plain, _ := st.InsertPrompt(ctx, dm, agent.ID, store.Prompt{Kind: "single", Question: "?", Options: []store.PromptOption{{Label: "a"}}})
	if resp, _ := c.do("POST", "/api/messages/"+plain.ID+"/connect", `{"credentials":{}}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("connect on a plain question: %d", resp.StatusCode)
	}
}

func TestConnectorSetupGuides(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"login":"martin"}`)) }))
	defer gh.Close()
	c, _ := setupWithKey(t)
	_, types := c.doList("GET", "/api/connectors/types")
	byName := map[string]map[string]any{}
	for _, raw := range types {
		ty := raw.(map[string]any)
		byName[ty["type"].(string)] = ty
	}

	// Slack's first step links to Slack's create-app page with openbot's manifest filled in.
	slack := byName["slack"]["setup"].(map[string]any)["steps"].([]any)
	link := slack[0].(map[string]any)["link"].(map[string]any)["url"].(string)
	u, err := url.Parse(link)
	if err != nil || u.Host != "api.slack.com" || u.Query().Get("new_app") != "1" {
		t.Fatalf("slack link = %s", link)
	}
	var manifest struct {
		Settings struct {
			SocketMode bool `json:"socket_mode_enabled"`
			Events     struct {
				Bot []string `json:"bot_events"`
			} `json:"event_subscriptions"`
		} `json:"settings"`
	}
	if err := json.Unmarshal([]byte(u.Query().Get("manifest_json")), &manifest); err != nil || !manifest.Settings.SocketMode ||
		len(manifest.Settings.Events.Bot) != 4 {
		t.Fatalf("manifest = %+v %v", manifest, err)
	}
	if slack[0].(map[string]any)["copy"] == nil {
		t.Fatal("the manifest should also be copyable")
	}

	// Event-only fields are flagged, and GitHub's webhook secret is made up by openbot.
	for _, f := range byName["github"]["credentialFields"].([]any) {
		field := f.(map[string]any)
		if field["key"] == "webhook_secret" && field["events"] != true {
			t.Fatal("webhook_secret should be an events field")
		}
	}
	resp, body := c.do("POST", "/api/connectors", `{"type":"github","name":"GitHub","credentials":{"token":"t"},"config":{"base_url":"`+gh.URL+`"}}`, true)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %v", resp.StatusCode, body)
	}
	if s, _ := body["webhookSecret"].(string); len(s) < 32 {
		t.Fatalf("webhookSecret = %v", body["webhookSecret"])
	}
}

func TestSignInAPI(t *testing.T) {
	ctx := context.Background()
	srv := oauthtest.New(t, true) // refuses plain http, except on loopback
	c, st := setupWithKey(t)
	agent, dm, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	noRedirect := &http.Client{Jar: c.http.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// Connecting it with a key says it uses sign-in.
	resp, body := c.do("POST", "/api/connectors", `{"type":"mcp","name":"Linear","credentials":{},"config":{"url":"`+srv.URL+`/mcp"}}`, true)
	if resp.StatusCode != http.StatusBadRequest || body["code"] != "sign_in_required" {
		t.Fatalf("key connect: %d %v", resp.StatusCode, body)
	}

	// A connect card: openbot's own address is loopback here, so the service sends the browser
	// straight back to /oauth/callback, which answers the card and returns to the chat.
	card, _ := st.InsertPrompt(ctx, dm, agent.ID, store.Prompt{Kind: "connect", Question: "Connect Linear?",
		Options: []store.PromptOption{{Label: "Connect"}, {Label: "Decline"}},
		Connection: &store.PendingConnection{AgentID: agent.ID, Type: "mcp", Name: "Linear", SignIn: true,
			Config: map[string]string{"url": srv.URL + "/mcp"}, AgentIDs: []string{agent.ID}}})
	resp, body = c.do("POST", "/api/connectors/sign-in", `{"messageId":"`+card.ID+`"}`, true)
	if resp.StatusCode != http.StatusOK || body["pasteBack"] != false {
		t.Fatalf("start: %d %v", resp.StatusCode, body)
	}
	landed := srv.Approve(t, body["authorizeUrl"].(string))
	if !strings.HasPrefix(landed, c.base+"/oauth/callback?") {
		t.Fatalf("landed on %s", landed)
	}
	r, err := noRedirect.Get(landed)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	loc, _ := url.Parse(r.Header.Get("Location"))
	if r.StatusCode != http.StatusSeeOther || loc.Path != "/chats/"+dm || loc.Query().Get("connected") == "" {
		t.Fatalf("callback redirect: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
	msg, _ := st.GetMessage(ctx, card.ID)
	if msg.Prompt.Status != "answered" || msg.Prompt.Connection.AccountID != loc.Query().Get("connected") {
		t.Fatalf("card = %+v", msg.Prompt)
	}
	_, list := c.doList("GET", "/api/connectors")
	if len(list) != 1 || list[0].(map[string]any)["signIn"] != "ok" {
		t.Fatalf("connectors = %v", list)
	}

	// Opened over plain http on a tailnet address, the service refuses openbot's address: the
	// user lands on an error page and pastes its address back.
	req, _ := http.NewRequest("POST", c.base+"/api/connectors/sign-in", strings.NewReader(`{"url":"`+srv.URL+`/mcp","name":"Notion","agentIds":["`+agent.ID+`"]}`))
	req.Host = "100.64.0.1:8080"
	base, _ := url.Parse(c.base)
	for _, ck := range c.http.Jar.Cookies(base) {
		req.AddCookie(ck)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, "1")
	r, err = c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var started map[string]any
	json.NewDecoder(r.Body).Decode(&started)
	r.Body.Close()
	if started["pasteBack"] != true {
		t.Fatalf("expected paste-back: %v", started)
	}
	pasted := srv.Approve(t, started["authorizeUrl"].(string))
	resp, body = c.do("POST", "/api/connectors/sign-in/complete", `{"callbackUrl":"`+pasted+`"}`, true)
	if resp.StatusCode != http.StatusOK || body["connector"].(map[string]any)["name"] != "Notion" || body["chatId"] != nil {
		t.Fatalf("complete: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("POST", "/api/connectors/sign-in/complete", `{"callbackUrl":"`+pasted+`"}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pasting twice should fail, got %d", resp.StatusCode)
	}

	// The service reporting a refusal goes back with a message.
	resp, body = c.do("POST", "/api/connectors/sign-in", `{"url":"`+srv.URL+`/mcp","name":"X"}`, true)
	authURL, _ := url.Parse(body["authorizeUrl"].(string))
	r, _ = noRedirect.Get(c.base + "/oauth/callback?error=access_denied&state=" + url.QueryEscape(authURL.Query().Get("state")))
	r.Body.Close()
	loc, _ = url.Parse(r.Header.Get("Location"))
	if loc.Path != "/connectors" || loc.Query().Get("signin_error") != "Sign-in was cancelled" {
		t.Fatalf("denied redirect: %s", r.Header.Get("Location"))
	}
}

func TestDeleteAgentAPI(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	admin, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "openbot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask", IsAdmin: true})
	helper, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Tracker", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	if resp, _ := c.do("DELETE", "/api/agents/"+helper.ID, "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := c.do("DELETE", "/api/agents/"+helper.ID, "", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete again: %d", resp.StatusCode)
	}
	if resp, body := c.do("DELETE", "/api/agents/"+admin.ID, "", true); resp.StatusCode != http.StatusConflict || !strings.Contains(body["message"].(string), "admin") {
		t.Fatalf("last admin: %d %v", resp.StatusCode, body)
	}
	if resp, _ := c.do("DELETE", "/api/agents/"+admin.ID, "", false); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without the CSRF header: %d", resp.StatusCode)
	}
}

// upload posts a file to a chat's attachments.
func (c *client) upload(chatID, name string, data []byte) (*http.Response, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/chats/"+chatID+"/attachments", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(csrfHeader, "1")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAttachmentsAPI(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	_, dm, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	_, other, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "b", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})

	// A PNG: type sniffed, dimensions read.
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	var png bytes.Buffer
	pngenc.Encode(&png, img)
	resp, shot := c.upload(dm, "../../screen shot.png", png.Bytes())
	if resp.StatusCode != http.StatusCreated || shot["mime"] != "image/png" || shot["width"] != float64(40) ||
		shot["height"] != float64(30) || shot["name"] != "screen shot.png" {
		t.Fatalf("upload: %d %v", resp.StatusCode, shot)
	}
	// HTML is stored, but never served as a page.
	_, page := c.upload(dm, "evil.html", []byte("<script>alert(1)</script>"))

	// Sending with attachments (and an empty body).
	body := `{"body":"","attachmentIds":["` + shot["id"].(string) + `","` + page["id"].(string) + `"]}`
	resp, msg := c.do("POST", "/api/chats/"+dm+"/messages", body, true)
	atts, _ := msg["attachments"].([]any)
	if resp.StatusCode != http.StatusCreated || len(atts) != 2 || atts[0].(map[string]any)["name"] != "screen shot.png" {
		t.Fatalf("send: %d %v", resp.StatusCode, msg)
	}
	// An upload is sent once, and only in its own chat.
	if resp, _ := c.do("POST", "/api/chats/"+dm+"/messages", body, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("resend: %d", resp.StatusCode)
	}
	_, stray := c.upload(dm, "x.txt", []byte("hi"))
	if resp, _ := c.do("POST", "/api/chats/"+other+"/messages", `{"body":"x","attachmentIds":["`+stray["id"].(string)+`"]}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("other chat: %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/chats/"+dm+"/messages", `{"body":""}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty message: %d", resp.StatusCode)
	}

	// Serving: images inline, HTML as a download with a harmless type.
	get := func(url string) *http.Response {
		r, err := c.http.Get(c.base + url)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r
	}
	r := get(shot["url"].(string))
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("image: %d %v", r.StatusCode, r.Header)
	}
	r = get(page["url"].(string))
	if r.Header.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") ||
		r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("html: %v", r.Header)
	}
	if r := get(shot["url"].(string) + "?download=1"); !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("download=1 should force a download")
	}
	anon, _ := http.Get(c.base + shot["url"].(string))
	if anon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("files need the session: %d", anon.StatusCode)
	}

	// Over 25 MB is refused.
	if resp, _ := c.upload(dm, "big.bin", make([]byte, attachments.MaxSize+1)); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", resp.StatusCode)
	}
}

func TestSidebarLayoutAPI(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	_, dmA, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Claude Sessions", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	_, dmB, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Ona Tester", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	_, dmC, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Grok Bot", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})

	_, casino := c.do("POST", "/api/sidebar/categories", `{"name":" casino "}`, true)
	_, groups := c.do("POST", "/api/sidebar/categories", `{"name":"groups"}`, true)
	if casino["name"] != "casino" || groups["id"] == nil {
		t.Fatalf("create: %v %v", casino, groups)
	}
	if resp, _ := c.do("POST", "/api/sidebar/categories", `{"name":"   "}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatal("empty names are refused")
	}
	cid, gid := casino["id"].(string), groups["id"].(string)

	// Put both DMs in casino (B first), reorder categories.
	layout := `{"categoryOrder":["` + gid + `","` + cid + `"],"sections":[{"categoryId":"` + cid + `","chatIds":["` + dmB + `","` + dmA + `"]}]}`
	if resp, body := c.do("PUT", "/api/sidebar/layout", layout, true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("layout: %d %v", resp.StatusCode, body)
	}
	_, cats := c.doList("GET", "/api/sidebar/categories")
	if len(cats) != 2 || cats[0].(map[string]any)["name"] != "groups" {
		t.Fatalf("categories = %v", cats)
	}
	chats := map[string]map[string]any{}
	_, list := c.doList("GET", "/api/chats")
	for _, raw := range list {
		ch := raw.(map[string]any)
		chats[ch["id"].(string)] = ch
	}
	if chats[dmB]["categoryId"] != cid || chats[dmB]["position"] != float64(0) || chats[dmA]["position"] != float64(1) ||
		chats[dmC]["categoryId"] != nil || chats[dmC]["position"] != nil {
		t.Fatalf("chats = %v / %v / %v", chats[dmA], chats[dmB], chats[dmC])
	}

	// Bad layouts: a category left out, an unknown chat, an unknown category.
	for _, bad := range []string{
		`{"categoryOrder":["` + cid + `"],"sections":[]}`,
		`{"categoryOrder":["` + gid + `","` + cid + `"],"sections":[{"categoryId":null,"chatIds":["nope"]}]}`,
		`{"categoryOrder":["` + gid + `","` + cid + `"],"sections":[{"categoryId":"nope","chatIds":["` + dmC + `"]}]}`,
	} {
		if resp, _ := c.do("PUT", "/api/sidebar/layout", bad, true); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("bad layout accepted: %s", bad)
		}
	}

	// Rename and collapse; deleting a category sends its chats back to Unassigned.
	resp, upd := c.do("PATCH", "/api/sidebar/categories/"+cid, `{"name":"Casino","collapsed":true}`, true)
	if resp.StatusCode != http.StatusOK || upd["name"] != "Casino" || upd["collapsed"] != true {
		t.Fatalf("update: %v", upd)
	}
	if resp, _ := c.do("DELETE", "/api/sidebar/categories/"+cid, "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	_, list = c.doList("GET", "/api/chats")
	for _, raw := range list {
		if ch := raw.(map[string]any); ch["categoryId"] != nil || ch["position"] != nil {
			t.Fatalf("after deleting the category: %v", ch)
		}
	}
}

func TestReplies(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	_, dm, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	_, other, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "b", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})

	long := strings.Repeat("é", 400)
	_, first := c.do("POST", "/api/chats/"+dm+"/messages", `{"body":"`+long+`"}`, true)
	resp, reply := c.do("POST", "/api/chats/"+dm+"/messages", `{"body":"yes, this","replyToId":"`+first["id"].(string)+`"}`, true)
	q, _ := reply["replyTo"].(map[string]any)
	if resp.StatusCode != http.StatusCreated || q == nil || q["id"] != first["id"] || q["available"] != true ||
		q["body"] != strings.Repeat("é", 300)+"…" || q["author"].(map[string]any)["kind"] != "user" {
		t.Fatalf("reply: %d %v", resp.StatusCode, reply)
	}
	_, page := c.do("GET", "/api/chats/"+dm+"/messages", "", false)
	msgs := page["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	if last["replyTo"].(map[string]any)["id"] != first["id"] || msgs[0].(map[string]any)["replyTo"] != nil {
		t.Fatalf("listed: %v", msgs)
	}
	// Only to messages in the same chat.
	for _, id := range []string{"nope", first["id"].(string)} {
		if resp, _ := c.do("POST", "/api/chats/"+other+"/messages", `{"body":"x","replyToId":"`+id+`"}`, true); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("reply to %s from another chat: %d", id, resp.StatusCode)
		}
	}
}

func TestClearChatHistory(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	a, dm, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	b, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "b", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	c.do("POST", "/api/chats/"+dm+"/messages", `{"body":"secret"}`, true)
	group, err := st.CreateGroup(ctx, "crew", []string{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}

	if resp, _ := c.do("DELETE", "/api/chats/"+dm+"/history", "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("clear: %d", resp.StatusCode)
	}
	if _, page := c.do("GET", "/api/chats/"+dm+"/messages", "", false); len(page["messages"].([]any)) != 0 {
		t.Fatalf("messages left: %v", page)
	}
	if resp, _ := c.do("DELETE", "/api/chats/"+group.ID+"/history", "", true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("clearing a group: %d", resp.StatusCode)
	}
	if resp, _ := c.do("DELETE", "/api/chats/missing/history", "", true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown chat: %d", resp.StatusCode)
	}
}

func TestEditMemoriesAndTasks(t *testing.T) {
	c, st := setupWithKey(t)
	ctx := context.Background()
	a, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	base := "/api/agents/" + a.ID

	// Memories: add, rewrite, validate.
	resp, mem := c.do("POST", base+"/memories", `{"text":"  Prefers short replies. "}`, true)
	if resp.StatusCode != http.StatusCreated || mem["text"] != "Prefers short replies." {
		t.Fatalf("add memory: %d %v", resp.StatusCode, mem)
	}
	resp, mem = c.do("PATCH", base+"/memories/"+mem["id"].(string), `{"text":"Prefers very short replies."}`, true)
	if resp.StatusCode != http.StatusOK || mem["text"] != "Prefers very short replies." {
		t.Fatalf("edit memory: %d %v", resp.StatusCode, mem)
	}
	if resp, _ := c.do("PATCH", base+"/memories/"+mem["id"].(string), `{"text":"   "}`, true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty memory: %d", resp.StatusCode)
	}
	if resp, _ := c.do("PATCH", base+"/memories/nope", `{"text":"x"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown memory: %d", resp.StatusCode)
	}
	if got, _ := st.Memories(ctx, a.ID); len(got) != 1 || got[0].Text != "Prefers very short replies." {
		t.Fatalf("stored: %+v", got)
	}

	// Tasks: create, validate, edit, pause.
	resp, task := c.do("POST", base+"/tasks", `{"name":"Check-in","purpose":"Ask how it's going","cron":"0 9 * * 1-5"}`, true)
	if resp.StatusCode != http.StatusCreated || task["kind"] != "cron" || task["nextFireAt"] == nil || task["enabled"] != true {
		t.Fatalf("create task: %d %v", resp.StatusCode, task)
	}
	for _, bad := range []string{
		`{"name":"x","purpose":"y"}`,
		`{"name":"x","purpose":"y","cron":"nope"}`,
		`{"name":"x","purpose":"y","at":"2001-01-01 10:00"}`,
		`{"name":"x","purpose":"y","cron":"0 9 * * *","at":"2099-01-01 10:00"}`,
	} {
		if resp, body := c.do("POST", base+"/tasks", bad, true); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: %d %v", bad, resp.StatusCode, body)
		}
	}
	id := task["id"].(string)
	resp, task = c.do("PATCH", "/api/tasks/"+id, `{"name":"Morning check-in","at":"2099-01-01 08:00"}`, true)
	if resp.StatusCode != http.StatusOK || task["name"] != "Morning check-in" || task["kind"] != "once" || task["purpose"] != "Ask how it's going" {
		t.Fatalf("edit task: %d %v", resp.StatusCode, task)
	}
	if resp, task = c.do("PATCH", "/api/tasks/"+id, `{"enabled":false}`, true); resp.StatusCode != http.StatusOK || task["enabled"] != false {
		t.Fatalf("pause: %d %v", resp.StatusCode, task)
	}
	if resp, _ := c.do("PATCH", "/api/tasks/nope", `{"enabled":true}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown task: %d", resp.StatusCode)
	}
	if resp, _ := c.do("POST", "/api/agents/nope/tasks", `{"name":"x","purpose":"y","cron":"0 9 * * *"}`, true); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown agent: %d", resp.StatusCode)
	}
}

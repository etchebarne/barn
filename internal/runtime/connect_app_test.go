package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/etchebarne/openbot/internal/connectors"
	"github.com/etchebarne/openbot/internal/connectors/oauthtest"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/store"
)

// An agent proposes a connection, the user adds the token on the card, and the agent gets the
// new tools without ever seeing the token.
func TestAgentProposesConnection(t *testing.T) {
	ctx := context.Background()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ghp_right" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return
		}
		fmt.Fprint(w, `{"login":"martin"}`)
	}))
	defer gh.Close()

	f := setup(t)
	box, _ := secrets.Open(t.TempDir())
	conns := connectors.NewManager(f.store, box)
	f.rt.Connectors = conns

	var mu sync.Mutex
	var results []string
	var sawTool bool
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		text := last.Text()
		for _, tool := range req.Tools {
			if tool.Function.Name == "my_github__list_issues" {
				mu.Lock()
				sawTool = true
				mu.Unlock()
			}
		}
		switch {
		case last.Role == "user" && strings.Contains(text, "hook up github"):
			return toolCall(toolConnectApp, map[string]any{"type": "github", "name": "My GitHub",
				"config": map[string]string{"base_url": gh.URL}, "reason": "So I can watch your PRs."})
		case last.Role == "user" && strings.Contains(text, "<connection_result"):
			mu.Lock()
			results = append(results, text)
			mu.Unlock()
		}
		return model.Text("assistant", "")
	}

	propose := func() store.Message {
		f.userSays(t, "hook up github")
		f.waitIdle(t)
		last, _ := f.store.LastMessage(ctx, f.chatID)
		if last.Prompt == nil || last.Prompt.Kind != "connect" {
			t.Fatalf("expected a connect card, got %+v", last)
		}
		return last
	}

	// Declining tells the agent and connects nothing.
	card := propose()
	if c := card.Prompt.Connection; c.Type != "github" || c.Config["base_url"] != gh.URL || len(c.AgentIDs) != 1 || c.AgentIDs[0] != f.agent.ID {
		t.Fatalf("card connection = %+v", c)
	}
	if !strings.Contains(card.Prompt.Question, "Connect My GitHub (GitHub)?\nSo I can watch your PRs.") {
		t.Fatalf("question = %q", card.Prompt.Question)
	}
	answerApproval(t, f, card, false)
	f.waitIdle(t)
	mu.Lock()
	if len(results) != 1 || !strings.Contains(results[0], `"declined":true`) {
		t.Fatalf("results = %v", results)
	}
	mu.Unlock()

	// A wrong token is rejected and the card stays open for another try.
	card = propose()
	if _, err := f.rt.Connect(ctx, card.ID, "", map[string]string{"token": "ghp_wrong"}); err == nil {
		t.Fatal("a wrong token should be rejected")
	}
	if accts, _ := f.store.Accounts(ctx); len(accts) != 0 {
		t.Fatal("nothing should be saved after a failed check")
	}
	msg, err := f.rt.Connect(ctx, card.ID, "", map[string]string{"token": "ghp_right"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Prompt.Status != "answered" || msg.Prompt.Connection.AccountID == "" {
		t.Fatalf("prompt = %+v", msg.Prompt)
	}
	if _, err := f.rt.Connect(ctx, card.ID, "", map[string]string{"token": "ghp_right"}); err != store.ErrPromptClosed {
		t.Fatalf("connecting twice: %v", err)
	}
	f.waitIdle(t)
	mu.Lock()
	defer mu.Unlock()
	if len(results) != 2 || !strings.Contains(results[1], `"connected":true`) || !strings.Contains(results[1], `"my_github__list_issues"`) {
		t.Fatalf("results = %v", results)
	}
	if !sawTool {
		t.Fatal("the agent should get the new tools")
	}
	for _, r := range results {
		if strings.Contains(r, "ghp_right") {
			t.Fatal("the agent must never see the token")
		}
	}
	accts, _ := f.store.Accounts(ctx)
	grants, _ := f.store.Grants(ctx, accts[0].ID)
	if len(accts) != 1 || accts[0].Name != "My GitHub" || len(grants) != 1 || grants[0] != f.agent.ID {
		t.Fatalf("accounts = %+v grants = %v", accts, grants)
	}
}

func TestConnectAppValidates(t *testing.T) {
	f := setup(t)
	box, _ := secrets.Open(t.TempDir())
	f.rt.Connectors = connectors.NewManager(f.store, box)
	l := &loop{m: f.rt, agentID: f.agent.ID}
	other, _, err := f.store.CreateAgentWithDM(context.Background(), store.Agent{Name: "Other", Model: "m", TrustMode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	agent := f.agent
	agent.IsAdmin = false
	for _, tc := range []struct{ args, want string }{
		{`{"type":"nope"}`, "unknown type"},
		{`{"type":"mcp"}`, "config.url is required"},
		{`{"type":"mcp","config":{"url":"ftp://x"}}`, "must start with https://"},
		{`{"type":"mcp","config":{"url":"https://x","token":"t"}}`, "no config field"},
		{`{"type":"webhook","agent_ids":["` + other.ID + `"]}`, "only propose connections for yourself"},
	} {
		out, ok := l.connectApp(context.Background(), agent, []byte(tc.args))
		if ok || !strings.Contains(out, tc.want) {
			t.Errorf("%s: got %s, want %q", tc.args, out, tc.want)
		}
	}
}

// Proposing an MCP server that uses sign-in gives a card with a Sign in button.
func TestConnectAppDetectsSignIn(t *testing.T) {
	ctx := context.Background()
	srv := oauthtest.New(t, false)
	f := setup(t)
	box, _ := secrets.Open(t.TempDir())
	f.rt.Connectors = connectors.NewManager(f.store, box)
	l := &loop{m: f.rt, agentID: f.agent.ID}
	if out, ok := l.connectApp(ctx, f.agent, []byte(`{"type":"mcp","name":"Linear","config":{"url":"`+srv.URL+`/mcp"}}`)); !ok {
		t.Fatal(out)
	}
	last, _ := f.store.LastMessage(ctx, f.chatID)
	if last.Prompt == nil || last.Prompt.Connection == nil || !last.Prompt.Connection.SignIn {
		t.Fatalf("card = %+v", last.Prompt)
	}
	if tool := connectAppTool(); !strings.Contains(tool.Function.Description, "https://mcp.linear.app/mcp") {
		t.Fatal("the tool should list the sign-in catalog")
	}
}

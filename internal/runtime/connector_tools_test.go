package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/barn/internal/connectors"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/store"
)

func TestAgentUsesConnectedApps(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var created []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"martin"}`)
		case r.Method == "GET" && r.URL.Path == "/repos/acme/web/issues":
			fmt.Fprint(w, `[{"number":7,"title":"Fix login","state":"open","user":{"login":"ana"}}]`)
		case r.Method == "POST" && r.URL.Path == "/repos/acme/web/issues":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			created = append(created, body["title"].(string))
			mu.Unlock()
			fmt.Fprint(w, `{"number":9,"html_url":"u9"}`)
		}
	}))
	defer gh.Close()

	f := setup(t)
	dir := t.TempDir()
	box, _ := secrets.Open(dir)
	conns := connectors.NewManager(f.store, box)
	conns.OnSignal = f.rt.DeliverSignal
	f.rt.Connectors = conns
	ghAcct, err := conns.Save(ctx, "", "github", "GitHub", map[string]string{"token": "t"}, map[string]string{"base_url": gh.URL})
	if err != nil {
		t.Fatal(err)
	}
	hook, _ := conns.Save(ctx, "", "webhook", "Uptime", map[string]string{}, map[string]string{})
	f.store.SetGrants(ctx, ghAcct.ID, []string{f.agent.ID})
	f.store.SetGrants(ctx, hook.ID, []string{f.agent.ID})

	var listed, sawEvent string
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		text := last.Text()
		switch {
		case last.Role == "user" && strings.Contains(text, "what's open?"):
			if !strings.Contains(req.Messages[0].Text(), "GitHub (GitHub, account_id") {
				t.Errorf("connected apps should be in the system prompt")
			}
			return toolCall("github__list_issues", map[string]string{"repo": "acme/web"})
		case last.Role == "tool" && strings.Contains(text, "Fix login"):
			mu.Lock()
			listed = text
			mu.Unlock()
			return model.Text("assistant", "")
		case last.Role == "user" && strings.Contains(text, "file a bug"):
			return toolCall("github__create_issue", map[string]string{"repo": "acme/web", "title": "Checkout broken"})
		case last.Role == "user" && strings.Contains(text, "watch uptime"):
			return toolCall(toolTaskCreate, map[string]any{"name": "Outage", "purpose": "Tell Martin the site is down.",
				"on_signal": map[string]any{"account": "Uptime", "type": "webhook.received", "match": map[string]string{"status": "down"}}})
		case last.Role == "user" && strings.Contains(text, "<task_fired"):
			mu.Lock()
			sawEvent = text
			mu.Unlock()
			return sendCall(f.chatID, "The site is down!")
		}
		return model.Text("assistant", "")
	}

	// 1. Read-only tools run directly.
	f.userSays(t, "what's open?")
	f.waitIdle(t)
	mu.Lock()
	if !strings.Contains(listed, `"title":"Fix login"`) {
		t.Fatalf("tool result = %q", listed)
	}
	mu.Unlock()

	// 2. Tools that act on the user's behalf need approval.
	f.userSays(t, "file a bug")
	f.waitIdle(t)
	msgs, _, _ := f.store.ListMessages(ctx, f.chatID, "", 50)
	card := msgs[len(msgs)-1]
	if card.Prompt == nil || card.Prompt.Kind != "approval" || !strings.Contains(card.Prompt.Question, "use GitHub: create_issue") {
		t.Fatalf("expected an approval card, got %+v", card)
	}
	mu.Lock()
	if len(created) != 0 {
		t.Fatal("nothing may be created before approval")
	}
	mu.Unlock()
	answerApproval(t, f, card, true)
	f.waitIdle(t)
	mu.Lock()
	if len(created) != 1 || created[0] != "Checkout broken" {
		t.Fatalf("approved call should run once, created %v", created)
	}
	mu.Unlock()

	// 3. A signal task wakes the agent when a matching event arrives.
	f.userSays(t, "watch uptime for me")
	f.waitIdle(t)
	tasks, _ := f.store.Tasks(ctx, f.agent.ID)
	if len(tasks) != 1 || tasks[0].Kind != "signal" || tasks[0].SignalAccountID != hook.ID {
		t.Fatalf("expected a signal task, got %+v", tasks)
	}
	token, _ := conns.Secret(ctx, hook.ID, "token")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{accountID}", conns.ServeWebhook)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, body := range []string{`{"status":"up"}`, `{"status":"down","site":"barn.dev"}`} {
		resp, err := http.Post(srv.URL+"/hooks/"+hook.ID+"?token="+token, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		got := sawEvent
		mu.Unlock()
		if got != "" {
			if !strings.Contains(got, `"site": "barn.dev"`) || !strings.Contains(got, "Tell Martin the site is down.") {
				t.Fatalf("task_fired should carry the event: %s", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the signal task never fired")
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.waitIdle(t)
	last, _ := f.store.LastMessage(ctx, f.chatID)
	if last.Body != "The site is down!" {
		t.Fatalf("last message = %q", last.Body)
	}
	all, _ := f.store.Tasks(ctx, f.agent.ID)
	if all[0].LastFiredAt == nil {
		t.Fatal("the task should record when it fired")
	}
}

// A signal task only fires for its own account, only while the agent has access, and goes away
// with the account.
func TestSignalTasksFollowTheirAccount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	box, _ := secrets.Open(t.TempDir())
	conns := connectors.NewManager(f.store, box)
	f.rt.Connectors = conns
	a, _ := conns.Save(ctx, "", "webhook", "A", map[string]string{}, map[string]string{})
	b, _ := conns.Save(ctx, "", "webhook", "B", map[string]string{}, map[string]string{})
	f.store.SetGrants(ctx, a.ID, []string{f.agent.ID})
	f.store.SetGrants(ctx, b.ID, []string{f.agent.ID})
	task, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: "A down", Purpose: "p", Kind: "signal",
		Enabled: true, SignalAccountID: a.ID, SignalType: "webhook.received", SignalMatch: map[string]string{"status": "down"}})
	if err != nil {
		t.Fatal(err)
	}
	fired := func() bool {
		got, err := f.store.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.LastFiredAt != nil
	}
	down := map[string]string{"status": "down"}
	f.rt.DeliverSignal(ctx, store.Signal{ID: "s1", AccountID: b.ID, Type: "webhook.received"}, down)
	if fired() {
		t.Fatal("a signal from another account fired the task")
	}
	f.store.SetGrants(ctx, a.ID, nil)
	f.rt.DeliverSignal(ctx, store.Signal{ID: "s2", AccountID: a.ID, Type: "webhook.received"}, down)
	if fired() {
		t.Fatal("fired without access to the account")
	}
	f.store.SetGrants(ctx, a.ID, []string{f.agent.ID})
	f.rt.DeliverSignal(ctx, store.Signal{ID: "s3", AccountID: a.ID, Type: "webhook.received"}, down)
	if !fired() {
		t.Fatal("matching signal didn't fire the task")
	}
	if err := conns.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetTask(ctx, task.ID); err == nil {
		t.Fatal("the task should be deleted with its account")
	}
}

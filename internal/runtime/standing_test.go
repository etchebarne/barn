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

	"github.com/etchebarne/barn/internal/connectors"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/store"
)

func TestStandingApprovals(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var created []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"martin"}`)
		case r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			created = append(created, r.URL.Path+" "+body["title"].(string))
			mu.Unlock()
			fmt.Fprint(w, `{"number":9,"html_url":"u9"}`)
		}
	}))
	defer gh.Close()
	f := setup(t)
	box, _ := secrets.Open(t.TempDir())
	conns := connectors.NewManager(f.store, box)
	f.rt.Connectors = conns
	acct, err := conns.Save(ctx, "", "github", "GitHub", map[string]string{"token": "t"}, map[string]string{"base_url": gh.URL})
	if err != nil {
		t.Fatal(err)
	}
	f.store.SetGrants(ctx, acct.ID, []string{f.agent.ID})

	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		text := last.Text()
		switch {
		case last.Role == "user" && strings.Contains(text, "file "):
			repo, title, _ := strings.Cut(strings.TrimSpace(text[strings.Index(text, "file ")+5:strings.Index(text, "\n</message>")]), " ")
			return toolCall("github__create_issue", map[string]string{"repo": repo, "title": title})
		case last.Role == "user" && strings.Contains(text, "stop asking for acme/web"):
			return toolCall(toolAllowWithoutAsking, map[string]any{"tool": "github__create_issue", "only_when": map[string]string{"repo": "acme/web"}})
		}
		return model.Text("assistant", "")
	}
	cards := func() int {
		msgs, _, _ := f.store.ListMessages(ctx, f.chatID, "", 100)
		n := 0
		for _, m := range msgs {
			if m.Prompt != nil && m.Prompt.Kind == "approval" {
				n++
			}
		}
		return n
	}
	lastCard := func() store.Message {
		last, _ := f.store.LastMessage(ctx, f.chatID)
		if last.Prompt == nil || last.Prompt.Kind != "approval" {
			t.Fatalf("expected an approval card, got %+v", last)
		}
		return last
	}
	answer := func(card store.Message, choice int) {
		answered, err := f.store.UpdatePrompt(ctx, card.ID, func(p store.Prompt) (store.Prompt, error) {
			p.Status, p.Answer = "answered", &store.PromptAnswer{Selected: []int{choice}}
			return p, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.rt.ResolveApproval(ctx, answered); err != nil {
			t.Fatal(err)
		}
		f.waitIdle(t)
	}

	// 1. A scoped standing approval, proposed by the agent when the user says so, confirmed
	// on a card (which itself can't be always allowed).
	f.userSays(t, "stop asking for acme/web issues")
	f.waitIdle(t)
	card := lastCard()
	if len(card.Prompt.Options) != 2 || card.Prompt.Preview.Title != "Always allow" ||
		!strings.Contains(card.Prompt.Question, "when Repo is acme/web") {
		t.Fatalf("allow card = %+v / %+v", card.Prompt, card.Prompt.Preview)
	}
	answer(card, 0)

	f.userSays(t, "file acme/web Login broken")
	f.waitIdle(t)
	f.userSays(t, "file acme/api Rate limits")
	f.waitIdle(t)
	mu.Lock()
	if len(created) != 1 || created[0] != "/repos/acme/web/issues Login broken" {
		t.Fatalf("only acme/web should run without asking: %v", created)
	}
	mu.Unlock()

	// 2. "Always allow" on a regular card approves it and stops asking for that action.
	card = lastCard()
	if len(card.Prompt.Options) != 3 || card.Prompt.Options[2].Label != "Always allow" {
		t.Fatalf("approval card options = %+v", card.Prompt.Options)
	}
	answer(card, 2)
	before := cards()
	f.userSays(t, "file acme/docs Typos")
	f.waitIdle(t)
	mu.Lock()
	if len(created) != 3 || cards() != before {
		t.Fatalf("after Always allow: created %v, cards %d -> %d", created, before, cards())
	}
	mu.Unlock()

	rules, _ := f.store.ApprovalRules(ctx, f.agent.ID)
	if len(rules) != 2 || rules[0].Label != "New issue · GitHub, when Repo is acme/web" || rules[1].Label != "New issue · GitHub" {
		t.Fatalf("rules = %+v", rules)
	}
	// Revoking brings the question back; removing the connection removes its rules.
	f.store.RevokeApproval(ctx, f.agent.ID, rules[1].ID)
	f.userSays(t, "file acme/docs More typos")
	f.waitIdle(t)
	if cards() != before+1 {
		t.Fatal("after revoking, it should ask again")
	}
	conns.Delete(ctx, acct.ID)
	if rules, _ := f.store.ApprovalRules(ctx, f.agent.ID); len(rules) != 0 {
		t.Fatalf("rules should go with the connection: %+v", rules)
	}
}

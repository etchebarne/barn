package runtime

import (
	"context"
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
)

// The agent that sets up others sees every connection, and a new agent given access sees its
// apps from its very first turn.
func TestCreateAgentWithConnections(t *testing.T) {
	ctx := context.Background()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"login":"martin"}`) }))
	defer gh.Close()
	f := setup(t)
	f.store.SetAgentAdmin(ctx, f.agent.ID, true)
	box, _ := secrets.Open(t.TempDir())
	conns := connectors.NewManager(f.store, box)
	f.rt.Connectors = conns
	if _, err := conns.Save(ctx, "", "github", "Work GitHub", map[string]string{"token": "t"}, map[string]string{"base_url": gh.URL}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var creatorPrompt, firstPrompt, created string
	f.llm.handler = func(req model.Request) model.Message {
		sys := req.Messages[0].Text()
		last := req.Messages[len(req.Messages)-1]
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(sys, "You are Repo Watch,") {
			if firstPrompt == "" {
				firstPrompt = sys
			}
			return model.Text("assistant", "")
		}
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), "watch my repos"):
			creatorPrompt = sys
			return toolCall(toolCreateAgent, map[string]any{"name": "Repo Watch", "job": "Watch my repos.",
				"instructions": "Summarize new issues.", "model": "other-model", "connections": []string{"work github"}})
		case last.Role == "user" && strings.Contains(last.Text(), "bad connection"):
			return toolCall(toolCreateAgent, map[string]any{"name": "Nope", "job": "x", "instructions": "x", "model": "other-model", "connections": []string{"Gmail"}})
		case last.Role == "tool":
			created = last.Text()
		}
		return model.Text("assistant", "")
	}

	f.userSays(t, "watch my repos")
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		done := firstPrompt != ""
		mu.Unlock()
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.waitIdle(t)
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(creatorPrompt, "# All connections") || !strings.Contains(creatorPrompt, "Work GitHub (GitHub") ||
		!strings.Contains(creatorPrompt, "no agent uses it yet") {
		t.Fatalf("the creator should see every connection:\n%s", creatorPrompt)
	}
	if !strings.Contains(firstPrompt, "Work GitHub (GitHub, account_id") || strings.Contains(firstPrompt, "# All connections") {
		t.Fatalf("the new agent should see its app (and only its own) from its first turn:\n%s", firstPrompt)
	}

	// An unknown connection is a clear error listing the real ones, and nothing is created.
	created = ""
	mu.Unlock()
	f.userSays(t, "bad connection")
	f.waitIdle(t)
	mu.Lock()
	if !strings.Contains(created, `no connection \"Gmail\" (connections: Work GitHub)`) {
		t.Fatalf("result = %s", created)
	}
	if agents, _ := f.store.ListAgents(ctx); len(agents) != 2 {
		t.Fatalf("agents = %d", len(agents))
	}
}

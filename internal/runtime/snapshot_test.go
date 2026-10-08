package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

func TestSystemPromptStaysFrozen(t *testing.T) {
	ctx := context.Background()
	var prompts []string
	var lastUser string
	f := setup(t)
	f.llm.handler = func(req model.Request) model.Message {
		prompts = append(prompts, req.Messages[0].Text())
		lastUser = req.Messages[len(req.Messages)-1].Text()
		return model.Text("assistant", "")
	}
	f.userSays(t, "one")
	f.waitIdle(t)
	f.userSays(t, "two")
	f.waitIdle(t)
	if len(prompts) != 2 || prompts[0] != prompts[1] {
		t.Fatalf("the system prompt should be byte-identical between turns")
	}
	if strings.Contains(prompts[0], "Current time") || !strings.Contains(prompts[0], "This was written on") {
		t.Fatalf("the prompt should carry the date, not the time:\n%s", prompts[0])
	}

	// What it's built from changes: rebuilt.
	instructions := "Be very brief."
	if _, err := f.rt.UpdateAgent(ctx, f.agent.ID, store.AgentUpdate{Instructions: &instructions}); err != nil {
		t.Fatal(err)
	}
	f.userSays(t, "three")
	f.waitIdle(t)
	if prompts[2] == prompts[1] || !strings.Contains(prompts[2], "Be very brief.") {
		t.Fatalf("changing instructions should rebuild the prompt")
	}

	// Notices carry when they arrived, since the prompt has no time.
	if err := f.rt.Notify(ctx, f.agent.ID, "the user changed something"); err != nil {
		t.Fatal(err)
	}
	f.waitIdle(t)
	if !strings.Contains(lastUser, "<system_notice received_at=") {
		t.Fatalf("notice without a time: %s", lastUser)
	}
}

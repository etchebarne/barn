package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

func newTarget(t *testing.T, f fixture) store.Agent {
	t.Helper()
	target, _, err := f.store.CreateAgentWithDM(context.Background(), store.Agent{
		Name: "Notes", Instructions: "x", Model: "test-model", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.rt.AddAgent(target.ID)
	return target
}

func answerApproval(t *testing.T, f fixture, msg store.Message, approve bool) {
	t.Helper()
	choice := 1
	if approve {
		choice = 0
	}
	answered, err := f.store.UpdatePrompt(context.Background(), msg.ID, func(p store.Prompt) (store.Prompt, error) {
		p.Status, p.Answer = "answered", &store.PromptAnswer{Selected: []int{choice}}
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.ResolveApproval(context.Background(), answered); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveNeedsApproval(t *testing.T) {
	for _, approve := range []bool{true, false} {
		t.Run(map[bool]string{true: "approved", false: "declined"}[approve], func(t *testing.T) {
			f := setup(t)
			f.store.SetAgentAdmin(context.Background(), f.agent.ID, true)
			target := newTarget(t, f)

			var outcome string
			f.llm.handler = func(req model.Request) model.Message {
				if !strings.HasPrefix(req.Messages[0].Text(), "You are barn,") {
					return model.Text("assistant", "")
				}
				last := req.Messages[len(req.Messages)-1]
				switch {
				case last.Role == "user" && strings.Contains(last.Text(), "archive Notes"):
					return toolCall(toolArchiveAgent, map[string]string{"agent_id": target.ID})
				case last.Role == "user" && strings.Contains(last.Text(), "<approval_result"):
					outcome = last.Text()
				}
				return model.Text("assistant", "")
			}
			f.userSays(t, "please archive Notes")
			msgs := f.waitForMessages(t, 2)
			f.waitIdle(t)
			card := msgs[1]
			if card.Prompt == nil || card.Prompt.Kind != "approval" || !strings.Contains(card.Prompt.Question, "Archive Notes?") {
				t.Fatalf("expected an approval card, got %+v", card)
			}
			if a, _ := f.store.GetAgent(context.Background(), target.ID); a.ID == "" {
				t.Fatal("target vanished before approval")
			}
			if agents, _ := f.store.ListAgents(context.Background()); len(agents) != 2 {
				t.Fatal("nothing should be archived before the user approves")
			}

			answerApproval(t, f, card, approve)
			f.waitIdle(t)
			agents, _ := f.store.ListAgents(context.Background())
			if approve && len(agents) != 1 {
				t.Fatalf("expected Notes archived after approval, have %d agents", len(agents))
			}
			if !approve && len(agents) != 2 {
				t.Fatalf("declining must not archive, have %d agents", len(agents))
			}
			want := `"approved":` + map[bool]string{true: "true", false: "false"}[approve]
			if !strings.Contains(outcome, want) {
				t.Fatalf("agent should be told the outcome (%s), got %q", want, outcome)
			}
		})
	}
}

func TestTrustedAgentsSkipApproval(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.store.SetAgentAdmin(ctx, f.agent.ID, true)
	trusted := "trusted"
	f.store.UpdateAgent(ctx, f.agent.ID, store.AgentUpdate{TrustMode: &trusted})
	target := newTarget(t, f)
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		if strings.HasPrefix(req.Messages[0].Text(), "You are barn,") && last.Role == "user" {
			return toolCall(toolArchiveAgent, map[string]string{"agent_id": target.ID})
		}
		return model.Text("assistant", "")
	}
	f.userSays(t, "archive Notes")
	f.waitIdle(t)
	if agents, _ := f.store.ListAgents(ctx); len(agents) != 1 {
		t.Fatalf("a trusted agent should archive without asking, have %d agents", len(agents))
	}
	if msgs, _, _ := f.store.ListMessages(ctx, f.chatID, "", 10); len(msgs) != 1 {
		t.Fatalf("no approval card expected, got %+v", msgs)
	}
}

func TestUpdateAgentTool(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message {
			return toolCall(toolUpdateAgent, map[string]string{"name": "Barnaby", "language": "Spanish"})
		},
		func(req model.Request) model.Message {
			if tr := req.Messages[len(req.Messages)-1].Text(); !strings.Contains(tr, `"name":"Barnaby"`) {
				t.Errorf("unexpected tool result %s", tr)
			}
			return model.Text("assistant", "")
		},
		func(model.Request) model.Message {
			return toolCall(toolUpdateAgent, map[string]string{"agent_id": "someone-else", "name": "x"})
		},
		func(req model.Request) model.Message {
			if tr := req.Messages[len(req.Messages)-1].Text(); !strings.Contains(tr, "only change your own") {
				t.Errorf("non-admins must not change other agents, got %s", tr)
			}
			return model.Text("assistant", "")
		},
	)
	f.userSays(t, "call yourself Barnaby and speak Spanish")
	f.waitIdle(t)
	a, _ := f.store.GetAgent(context.Background(), f.agent.ID)
	if a.Name != "Barnaby" || a.Language != "Spanish" {
		t.Fatalf("expected renamed agent, got %+v", a)
	}
	if c, _ := f.store.GetChat(context.Background(), f.chatID); c.Name != "Barnaby" {
		t.Fatalf("the DM should follow the agent's name, got %q", c.Name)
	}
	f.userSays(t, "rename someone else")
	f.waitIdle(t)
}

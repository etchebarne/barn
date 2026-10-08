package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/sandbox"
)

func TestSecrets(t *testing.T) {
	const token = "ghp_Sup3r\"Secret/123"
	ctx := context.Background()
	f := setup(t)
	sbx := &fakeSandbox{exec: func(command string, env map[string]string) sandbox.Result {
		// A careless command that prints the token.
		return sandbox.Result{Output: "token is " + env["GITHUB_TOKEN"] + "\n"}
	}}
	f.rt.Sandboxes = sbx

	var lastSystem, result, answer string
	f.llm.handler = func(req model.Request) model.Message {
		lastSystem = req.Messages[0].Text()
		last := req.Messages[len(req.Messages)-1]
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), "need the github token"):
			return toolCall(toolRequestSecret, map[string]string{"name": "GITHUB_TOKEN", "description": "GitHub token with repo scope"})
		case last.Role == "user" && strings.Contains(last.Text(), "<secret_result"):
			answer = last.Text()
			return toolCall(toolRunCommand, map[string]string{"command": "echo token is $GITHUB_TOKEN"})
		case last.Role == "tool" && strings.Contains(last.Text(), "exit_code"):
			result = last.Text()
		}
		return model.Text("assistant", "")
	}

	f.userSays(t, "I need the github token set up")
	msgs := f.waitForMessages(t, 2)
	f.waitIdle(t)
	card := msgs[1]
	if card.Prompt == nil || card.Prompt.Kind != "secret" || card.Prompt.Secret == nil || card.Prompt.Secret.Name != "GITHUB_TOKEN" {
		t.Fatalf("expected a secret card, got %+v", card)
	}
	if n := f.llm.calls(); n != 1 {
		t.Fatalf("asking for a secret ends the turn: %d calls", n)
	}

	if _, err := f.rt.ProvideSecret(ctx, card.ID, "  "+token+"\n"); err != nil {
		t.Fatal(err)
	}
	f.waitIdle(t)
	if !strings.Contains(answer, `provided="true"`) || strings.Contains(answer, token) {
		t.Fatalf("agent should learn it's provided, never the value: %q", answer)
	}
	if !strings.Contains(lastSystem, "$GITHUB_TOKEN (GitHub token with repo scope)") {
		t.Fatalf("the system prompt should list the secret:\n%s", lastSystem)
	}
	if len(sbx.env) == 0 || sbx.env[len(sbx.env)-1]["GITHUB_TOKEN"] != token {
		t.Fatalf("run_command should get the secret as an env var (trimmed): %v", sbx.env)
	}
	if !strings.Contains(result, "token is [secret GITHUB_TOKEN]") || strings.Contains(result, "Sup3r") {
		t.Fatalf("output should be masked: %s", result)
	}

	// The value is nowhere the model or the chat can see.
	entries, _ := f.store.Context(ctx, f.agent.ID)
	for _, e := range entries {
		if strings.Contains(string(e.Entry), "Sup3r") {
			t.Fatalf("secret leaked into the context: %s", e.Entry)
		}
	}
	got, _ := f.store.GetMessage(ctx, card.ID)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "Sup3r") || got.Prompt.Status != "answered" {
		t.Fatalf("card: %s", raw)
	}

	// Answered once; bad names and empty values are refused.
	if _, err := f.rt.ProvideSecret(ctx, card.ID, "again"); err == nil {
		t.Fatal("a card takes one secret")
	}
	for _, name := range []string{"lower", "PATH", "X"} {
		if err := f.rt.SetSecret(ctx, f.agent.ID, name, "", "value"); err == nil {
			t.Fatalf("name %q should be refused", name)
		}
	}
	if err := f.rt.SetSecret(ctx, f.agent.ID, "OTHER", "", "   "); err == nil {
		t.Fatal("an empty secret should be refused")
	}
	list, _ := f.store.AgentSecrets(ctx, f.agent.ID)
	if len(list) != 1 || list[0].Description != "GitHub token with repo scope" {
		t.Fatalf("secrets: %+v", list)
	}
}

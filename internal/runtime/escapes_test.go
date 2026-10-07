package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
)

func TestFixEscapes(t *testing.T) {
	for in, want := range map[string]string{
		`two things:\nOUT — relay\nIN — watch`: "two things:\nOUT — relay\nIN — watch",
		`windows\r\nline`:                      "windows\nline",
		"already\nfine with a literal \\n":     "already\nfine with a literal \\n",
		"no escapes":                           "no escapes",
		"run `printf 'a\\nb'` then\\ndone":     "run `printf 'a\\nb'` then\ndone",
	} {
		if got := fixEscapes(in); got != want {
			t.Errorf("fixEscapes(%q) = %q, want %q", in, got, want)
		}
	}
	got := fixArgEscapes(`{"chat_id":"c1","text":"a\\nb","n":1}`, "text")
	if got != `{"chat_id":"c1","n":1,"text":"a\nb"}` {
		t.Fatalf("fixArgEscapes = %s", got)
	}
	if same := `{"text":"fine"}`; fixArgEscapes(same, "text") != same {
		t.Fatal("untouched arguments should be returned as they are")
	}
}

// A double-escaped message from the model arrives with real line breaks.
func TestSendMessageFixesEscapes(t *testing.T) {
	f := setup(t)
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "user" {
			return sendCall(f.chatID, `I'll take care of two things:\nOUT — relay\nIN — watch`)
		}
		return model.Text("assistant", "")
	}
	f.userSays(t, "what will you do?")
	f.waitIdle(t)
	last, _ := f.store.LastMessage(context.Background(), f.chatID)
	if last.Body != "I'll take care of two things:\nOUT — relay\nIN — watch" {
		t.Fatalf("body = %q", last.Body)
	}
}

// Agents can't react to their own messages (models sometimes pick their own message_id).
func TestNoReactingToOwnMessages(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	id := f.agent.ID
	own, _ := f.store.InsertMessage(ctx, f.chatID, "agent", &id, "I'll ping you only when needed.")
	theirs, _ := f.store.InsertMessage(ctx, f.chatID, "user", nil, "only ping me when needed")
	l := &loop{m: f.rt, agentID: id}
	if out, ok := l.react(ctx, f.agent, []byte(`{"message_id":"`+own.ID+`","emoji":"👀"}`)); ok || !strings.Contains(out, "your own message") {
		t.Fatalf("reacting to its own message: %s", out)
	}
	if _, ok := l.react(ctx, f.agent, []byte(`{"message_id":"`+theirs.ID+`","emoji":"👀"}`)); !ok {
		t.Fatal("reacting to the user's message should work")
	}
	if m, _ := f.store.GetMessage(ctx, own.ID); len(m.Reactions) != 0 {
		t.Fatal("no reaction should be stored on its own message")
	}
}

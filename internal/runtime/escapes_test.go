package runtime

import (
	"context"
	"testing"

	"github.com/etchebarne/barn/internal/model"
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

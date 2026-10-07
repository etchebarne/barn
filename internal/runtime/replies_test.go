package runtime

import (
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/store"
)

func TestRenderReply(t *testing.T) {
	names := map[string]string{"ag1": "Relay"}
	agent := "ag1"
	id := "m1"
	if got := renderReply(store.Message{Body: "hi"}, "martin", names); got != "" {
		t.Fatalf("not a reply: %q", got)
	}
	gone := renderReply(store.Message{ReplyTo: &id}, "martin", names)
	if !strings.Contains(gone, `message_id="m1"`) || !strings.Contains(gone, "deleted") {
		t.Fatalf("deleted: %q", gone)
	}
	q := &store.Message{ID: id, AuthorKind: "agent", AuthorAgentID: &agent, Body: strings.Repeat("x", 600),
		Attachments: []store.Attachment{{Name: "chart.png"}}}
	got := renderReply(store.Message{ReplyTo: &id, Quoted: q}, "martin", names)
	if !strings.HasPrefix(got, `<replying_to message_id="m1" from="Relay">`) || !strings.Contains(got, strings.Repeat("x", 500)+"…") ||
		strings.Contains(got, strings.Repeat("x", 501)) || !strings.Contains(got, "(attached: chart.png)") {
		t.Fatalf("reply: %q", got)
	}
}

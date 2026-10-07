package runtime

import (
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/store"
)

// replyQuoteLength is how much of a replied-to message the agent sees with the reply.
const replyQuoteLength = 500

// renderReply shows what msg replies to, ahead of its text ("" if it isn't a reply).
func renderReply(msg store.Message, username string, names map[string]string) string {
	if msg.ReplyTo == nil {
		return ""
	}
	q := msg.Quoted
	if q == nil {
		return fmt.Sprintf("<replying_to message_id=%q>(that message was deleted)</replying_to>\n", *msg.ReplyTo)
	}
	text := q.Body
	if r := []rune(text); len(r) > replyQuoteLength {
		text = string(r[:replyQuoteLength]) + "…"
	}
	if len(q.Attachments) > 0 {
		files := make([]string, len(q.Attachments))
		for i, a := range q.Attachments {
			files[i] = a.Name
		}
		text = strings.TrimSpace(text + "\n(attached: " + strings.Join(files, ", ") + ")")
	}
	return fmt.Sprintf("<replying_to message_id=%q from=%q>\n%s\n</replying_to>\n", q.ID, author(*q, username, names), text)
}

// author is who wrote a message, as agents see it.
func author(msg store.Message, username string, names map[string]string) string {
	switch {
	case msg.AuthorKind == "agent" && msg.AuthorAgentID != nil:
		return names[*msg.AuthorAgentID]
	case msg.AuthorKind == "system":
		return "system"
	}
	return username
}

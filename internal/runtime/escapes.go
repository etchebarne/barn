package runtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// fixEscapes undoes a common model mistake: double-escaping text inside tool arguments, so a
// message arrives with the two characters \n where line breaks were meant. It only acts on
// text with no real line breaks at all, and leaves `code` and ```blocks``` alone, where a
// literal \n may be intended.
func fixEscapes(s string) string {
	if strings.Contains(s, "\n") || !strings.Contains(s, `\n`) {
		return s
	}
	var b strings.Builder
	inCode := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '`':
			inCode = !inCode
			b.WriteByte(s[i])
		case !inCode && s[i] == '\\' && i+1 < len(s) && s[i+1] == 'n':
			b.WriteByte('\n')
			i++
		case !inCode && s[i] == '\\' && i+3 < len(s) && s[i+1] == 'r' && s[i+2] == '\\' && s[i+3] == 'n':
			b.WriteByte('\n')
			i += 3
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// fixProse repairs double-escaped line breaks in the arguments that are prose: messages,
// questions, memories, and the main text of connector actions (e.g. a Slack message).
func (l *loop) fixProse(ctx context.Context, agent store.Agent, call model.ToolCall) string {
	args := call.Function.Arguments
	switch call.Function.Name {
	case toolSendMessage, toolRemember:
		return fixArgEscapes(args, "text")
	case toolAskUser:
		return fixArgEscapes(args, "question")
	}
	if t, ok := l.connectorTool(ctx, agent, call.Function.Name); ok {
		body := t.Tool.Body
		if body == "" {
			body = guessBody(json.RawMessage(args))
		}
		if body != "" {
			return fixArgEscapes(args, body)
		}
	}
	return args
}

// fixArgEscapes applies fixEscapes to the given string fields of a tool call's arguments.
func fixArgEscapes(args string, fields ...string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(args), &m) != nil {
		return args
	}
	changed := false
	for _, f := range fields {
		var s string
		if raw, ok := m[f]; !ok || json.Unmarshal(raw, &s) != nil {
			continue
		}
		if fixed := fixEscapes(s); fixed != s {
			b, _ := json.Marshal(fixed)
			m[f], changed = b, true
		}
	}
	if !changed {
		return args
	}
	b, err := json.Marshal(m)
	if err != nil {
		return args
	}
	return string(b)
}

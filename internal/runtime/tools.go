package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

const toolSendMessage = "send_message"

func tools() []model.Tool {
	return []model.Tool{{
		Type: "function",
		Function: model.FunctionSpec{
			Name: toolSendMessage,
			Description: "Post a message in one of your chats. This is the only way anyone sees " +
				"what you write. Supports Markdown.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"chat_id": {"type": "string", "description": "The chat to post in (from your chat list)."},
					"text": {"type": "string", "description": "The message, in Markdown."}
				},
				"required": ["chat_id", "text"],
				"additionalProperties": false
			}`),
		},
	}}
}

func toolLabel(name string) string {
	switch name {
	case toolSendMessage:
		return "writing a message"
	default:
		return "using " + name
	}
}

// runTool executes a tool call and returns the result for the model, and whether it succeeded.
func (l *loop) runTool(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	switch call.Function.Name {
	case toolSendMessage:
		var args struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return toolError("invalid arguments: %v", err), false
		}
		if strings.TrimSpace(args.Text) == "" {
			return toolError("text is empty"), false
		}
		chat, err := l.m.store.GetChat(ctx, args.ChatID)
		if err != nil {
			return toolError("unknown chat_id %q", args.ChatID), false
		}
		if !slices.ContainsFunc(chat.Members, func(m store.ChatMember) bool { return m.AgentID == agent.ID }) {
			return toolError("you are not a member of chat %q", args.ChatID), false
		}
		msg, err := l.m.postMessage(ctx, chat.ID, "agent", &agent.ID, args.Text)
		if err != nil {
			logger(agent.ID).Error("send_message", "err", err)
			return toolError("failed to send the message"), false
		}
		return toolOK(map[string]string{"message_id": msg.ID}), true

	default:
		return toolError("unknown tool %q", call.Function.Name), false
	}
}

func toolOK(v any) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": v})
	return string(b)
}

func toolError(format string, args ...any) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": fmt.Sprintf(format, args...)})
	return string(b)
}

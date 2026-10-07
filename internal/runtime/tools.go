package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

const (
	toolSendMessage = "send_message"
	toolReact       = "react"
	toolAskUser     = "ask_user"
	toolListModels  = "list_models"
	toolCreateAgent = "create_agent"
)

const maxPromptOptions = 8

func function(name, description, parameters string) model.Tool {
	return model.Tool{Type: "function", Function: model.FunctionSpec{
		Name: name, Description: description, Parameters: json.RawMessage(parameters),
	}}
}

var (
	sendMessageTool = function(toolSendMessage,
		"Post a message in one of your chats. This is the only way anyone sees what you write. "+
			"Supports Markdown, including tables. You can call it several times in a row to send "+
			"several short messages, like people do in chat.",
		`{
			"type": "object",
			"properties": {
				"chat_id": {"type": "string", "description": "The chat to post in (from your chat list)."},
				"text": {"type": "string", "description": "The message, in Markdown."}
			},
			"required": ["chat_id", "text"],
			"additionalProperties": false
		}`)

	reactTool = function(toolReact,
		"React to a message with an emoji, like people do in chat. Use it when a message doesn't "+
			"need a written reply (thanks, an FYI, a done-update) or to acknowledge something "+
			"you're about to work on.",
		`{
			"type": "object",
			"properties": {
				"message_id": {"type": "string", "description": "The message to react to (message_id from its <message> tag)."},
				"emoji": {"type": "string", "description": "A single emoji, e.g. 👍 ❤️ 😂 🎉 👀 ✅ 🙏 🔥"}
			},
			"required": ["message_id", "emoji"],
			"additionalProperties": false
		}`)

	askUserTool = function(toolAskUser,
		"Ask the user a question they answer by clicking, instead of typing. Use it whenever "+
			"there are clear options. kind=single: pick one; kind=multi: pick any number; "+
			"kind=text: free-text answer (no options). Set allow_other to also offer \"type your "+
			"own\". Asking ends your turn: send anything else first, in the same reply. The answer "+
			"arrives later as a <prompt_answer>.",
		`{
			"type": "object",
			"properties": {
				"chat_id": {"type": "string", "description": "The chat to ask in."},
				"question": {"type": "string", "description": "Short question shown above the options."},
				"kind": {"type": "string", "enum": ["single", "multi", "text"]},
				"options": {
					"type": "array",
					"maxItems": 8,
					"description": "Answer options (required for single and multi). Keep labels short.",
					"items": {
						"type": "object",
						"properties": {
							"label": {"type": "string"},
							"opens_chat_id": {"type": "string", "description": "Optional: choosing this option also opens this chat in the user's app (e.g. \"Go talk to X\")."}
						},
						"required": ["label"],
						"additionalProperties": false
					}
				},
				"allow_other": {"type": "boolean", "description": "Also offer a \"type your own\" answer."}
			},
			"required": ["chat_id", "question", "kind"],
			"additionalProperties": false
		}`)

	listModelsTool = function(toolListModels,
		"List the model ids available from the user's OpenCode Go subscription.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	createAgentTool = function(toolCreateAgent,
		"Create a new agent: a persistent teammate that owns one job. It gets its own DM with the "+
			"user and introduces itself there right away. Ask the user which model to use first "+
			"(offer your own model as the default).",
		`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "Short, human name for the agent, e.g. \"Claude Sessions\"."},
				"job": {"type": "string", "description": "One sentence: the job this agent owns."},
				"instructions": {"type": "string", "description": "Detailed instructions for the agent: what it does, how, for whom, and any preferences the user mentioned."},
				"model": {"type": "string", "description": "Model id (from list_models)."}
			},
			"required": ["name", "job", "instructions", "model"],
			"additionalProperties": false
		}`)
)

// toolsFor returns the tools an agent may use.
func toolsFor(agent store.Agent) []model.Tool {
	tools := []model.Tool{sendMessageTool, reactTool, askUserTool}
	if agent.IsAdmin {
		tools = append(tools, listModelsTool, createAgentTool)
	}
	return tools
}

func toolLabel(name string) string {
	switch name {
	case toolSendMessage:
		return "writing a message"
	case toolReact:
		return "reacting"
	case toolAskUser:
		return "asking a question"
	case toolListModels:
		return "checking models"
	case toolCreateAgent:
		return "setting up an agent"
	default:
		return "using " + name
	}
}

// runTool executes a tool call and returns the result for the model, and whether it succeeded.
func (l *loop) runTool(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	args := []byte(call.Function.Arguments)
	if !slices.ContainsFunc(toolsFor(agent), func(t model.Tool) bool { return t.Function.Name == call.Function.Name }) {
		return toolError("unknown tool %q", call.Function.Name), false
	}
	switch call.Function.Name {
	case toolSendMessage:
		return l.sendMessage(ctx, agent, args)
	case toolReact:
		return l.react(ctx, agent, args)
	case toolAskUser:
		return l.askUser(ctx, agent, args)
	case toolListModels:
		models, err := l.m.llm.Models(ctx)
		if err != nil {
			return toolError("couldn't load models: %v", err), false
		}
		return toolOK(map[string]any{"models": models, "your_model": agent.Model}), true
	case toolCreateAgent:
		return l.createAgent(ctx, agent, args)
	default:
		return toolError("unknown tool %q", call.Function.Name), false
	}
}

// memberChat loads a chat the agent belongs to.
func (l *loop) memberChat(ctx context.Context, agent store.Agent, chatID string) (store.Chat, error) {
	chat, err := l.m.store.GetChat(ctx, chatID)
	if err != nil {
		return chat, fmt.Errorf("unknown chat_id %q", chatID)
	}
	if !slices.ContainsFunc(chat.Members, func(m store.ChatMember) bool { return m.AgentID == agent.ID }) {
		return chat, fmt.Errorf("you are not a member of chat %q", chatID)
	}
	return chat, nil
}

func (l *loop) sendMessage(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if strings.TrimSpace(args.Text) == "" {
		return toolError("text is empty"), false
	}
	chat, err := l.memberChat(ctx, agent, args.ChatID)
	if err != nil {
		return toolError("%v", err), false
	}
	msg, err := l.m.postMessage(ctx, chat.ID, "agent", &agent.ID, args.Text)
	if err != nil {
		logger(agent.ID).Error("send_message", "err", err)
		return toolError("failed to send the message"), false
	}
	return toolOK(map[string]string{"message_id": msg.ID}), true
}

func (l *loop) react(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	emoji := strings.TrimSpace(args.Emoji)
	if !isEmoji(emoji) {
		return toolError("emoji must be a single emoji, like 👍"), false
	}
	msg, err := l.m.store.GetMessage(ctx, args.MessageID)
	if err != nil {
		return toolError("unknown message_id %q", args.MessageID), false
	}
	if _, err := l.memberChat(ctx, agent, msg.ChatID); err != nil {
		return toolError("%v", err), false
	}
	added, err := l.m.store.AddReaction(ctx, msg.ID, "agent:"+agent.ID, emoji)
	if err != nil {
		logger(agent.ID).Error("react", "err", err)
		return toolError("failed to react"), false
	}
	if added {
		if updated, err := l.m.store.GetMessage(ctx, msg.ID); err == nil {
			l.m.bus.Publish(view.MessageUpdated(view.Message(updated)))
		}
	}
	return toolOK(map[string]string{"reacted": emoji}), true
}

// isEmoji is a loose check that s is one short emoji (possibly with modifiers or joiners):
// no letters, digits, spaces or other ASCII.
func isEmoji(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if r < 0x80 || unicode.IsSpace(r) || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (l *loop) askUser(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		ChatID   string `json:"chat_id"`
		Question string `json:"question"`
		Kind     string `json:"kind"`
		Options  []struct {
			Label       string `json:"label"`
			OpensChatID string `json:"opens_chat_id"`
		} `json:"options"`
		AllowOther bool `json:"allow_other"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	args.Question = strings.TrimSpace(args.Question)
	if args.Question == "" || utf8.RuneCountInString(args.Question) > 500 {
		return toolError("question must be 1–500 characters"), false
	}
	p := store.Prompt{Kind: args.Kind, Question: args.Question, AllowOther: args.AllowOther}
	switch args.Kind {
	case "single", "multi":
		if len(args.Options) == 0 || len(args.Options) > maxPromptOptions {
			return toolError("%s questions need 1–%d options", args.Kind, maxPromptOptions), false
		}
		for _, o := range args.Options {
			label := strings.TrimSpace(o.Label)
			if label == "" {
				return toolError("option labels can't be empty"), false
			}
			if o.OpensChatID != "" {
				if _, err := l.m.store.GetChat(ctx, o.OpensChatID); err != nil {
					return toolError("opens_chat_id %q is not a chat", o.OpensChatID), false
				}
			}
			p.Options = append(p.Options, store.PromptOption{Label: label, OpensChatID: o.OpensChatID})
		}
	case "text":
		if len(args.Options) > 0 {
			return toolError("text questions take no options"), false
		}
		p.AllowOther = false
	default:
		return toolError("kind must be single, multi, or text"), false
	}
	chat, err := l.memberChat(ctx, agent, args.ChatID)
	if err != nil {
		return toolError("%v", err), false
	}
	msg, err := l.m.store.InsertPrompt(ctx, chat.ID, agent.ID, p)
	if err != nil {
		logger(agent.ID).Error("ask_user", "err", err)
		return toolError("failed to ask the question"), false
	}
	l.m.publishMessage(msg)
	return toolOK(map[string]string{
		"prompt_id": msg.ID,
		"note":      "Asked. Your turn ends here; the answer will arrive as a <prompt_answer>.",
	}), true
}

func (l *loop) createAgent(ctx context.Context, creator store.Agent, raw []byte) (string, bool) {
	var args struct {
		Name         string `json:"name"`
		Job          string `json:"job"`
		Instructions string `json:"instructions"`
		Model        string `json:"model"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	created, chatID, err := l.m.CreateAgent(ctx, NewAgent{
		Name:         args.Name,
		Job:          args.Job,
		Instructions: args.Instructions,
		Model:        args.Model,
		CreatedBy:    &creator,
	})
	if err != nil {
		return toolError("%v", err), false
	}
	return toolOK(map[string]string{
		"agent_id": created.ID,
		"chat_id":  chatID,
		"note":     created.Name + " now exists and is introducing itself in its own DM (chat_id " + chatID + ").",
	}), true
}

func toolOK(v any) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": v})
	return string(b)
}

func toolError(format string, args ...any) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": fmt.Sprintf(format, args...)})
	return string(b)
}

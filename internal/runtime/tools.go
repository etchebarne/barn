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
	toolSendMessage  = "send_message"
	toolReact        = "react"
	toolAskUser      = "ask_user"
	toolListModels   = "list_models"
	toolListAgents   = "list_agents"
	toolRemember     = "memory_save"
	toolForget       = "memory_forget"
	toolCreateAgent  = "create_agent"
	toolUpdateAgent  = "update_agent"
	toolArchiveAgent = "archive_agent"
	toolCreateGroup  = "create_group"
	toolUpdateGroup  = "update_group"
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

	rememberTool = function(toolRemember,
		"Save a durable memory: a fact or preference you must not lose (your chat history gets "+
			"summarized over time). One short, self-contained sentence, e.g. \"Martin prefers replies in lowercase.\"",
		`{
			"type": "object",
			"properties": {
				"text": {"type": "string", "description": "The memory, one short sentence."},
				"chat_id": {"type": "string", "description": "Optional: the chat where you learned it."}
			},
			"required": ["text"],
			"additionalProperties": false
		}`)

	forgetTool = function(toolForget,
		"Delete one of your memories (when it's wrong or no longer true). Use the id shown in your memories.",
		`{
			"type": "object",
			"properties": {"memory_id": {"type": "string"}},
			"required": ["memory_id"],
			"additionalProperties": false
		}`)

	listAgentsTool = function(toolListAgents,
		"List the agents in barn (your teammates): their agent_id, name, what they do, and model.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	listModelsTool = function(toolListModels,
		"List the model ids available from the user's OpenCode Go subscription.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	updateAgentTool = function(toolUpdateAgent,
		"Change an agent's settings: rename it, rewrite its instructions, or switch its model or "+
			"reply language. Leave agent_id out to change yourself. Only set the fields that change.",
		`{
			"type": "object",
			"properties": {
				"agent_id": {"type": "string", "description": "The agent to change; defaults to you."},
				"name": {"type": "string"},
				"instructions": {"type": "string", "description": "The full new instructions (they replace the old ones)."},
				"model": {"type": "string", "description": "Model id (from list_models)."},
				"language": {"type": "string", "description": "\"auto\" or a language name, e.g. \"Spanish\"."},
				"sandbox_with": {"type": "string", "description": "Admins only: an agent_id whose computer this agent should share from now on."}
			},
			"additionalProperties": false
		}`)

	archiveAgentTool = function(toolArchiveAgent,
		"Archive an agent: it stops working and its chat is hidden (history is kept). Needs the "+
			"user's approval unless you're trusted. You can't archive yourself.",
		`{
			"type": "object",
			"properties": {"agent_id": {"type": "string"}},
			"required": ["agent_id"],
			"additionalProperties": false
		}`)

	createGroupTool = function(toolCreateGroup,
		"Create a group chat between the user and two or more agents, so they can coordinate in one "+
			"place (agents take turns there). Use list_agents for ids; include yourself if you should take part.",
		`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "Short group name, e.g. \"Launch crew\"."},
				"agent_ids": {"type": "array", "items": {"type": "string"}, "minItems": 2}
			},
			"required": ["name", "agent_ids"],
			"additionalProperties": false
		}`)

	updateGroupTool = function(toolUpdateGroup,
		"Rename a group chat, or add or remove agents.",
		`{
			"type": "object",
			"properties": {
				"chat_id": {"type": "string"},
				"name": {"type": "string"},
				"add_agent_ids": {"type": "array", "items": {"type": "string"}},
				"remove_agent_ids": {"type": "array", "items": {"type": "string"}}
			},
			"required": ["chat_id"],
			"additionalProperties": false
		}`)

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
				"model": {"type": "string", "description": "Model id (from list_models)."},
				"sandbox_with": {"type": "string", "description": "Optional agent_id: share that agent's computer instead of getting a new one (for agents that work on the same files)."}
			},
			"required": ["name", "job", "instructions", "model"],
			"additionalProperties": false
		}`)
)

// toolsFor returns the tools an agent may use.
func toolsFor(agent store.Agent, sandboxes bool) []model.Tool {
	tools := []model.Tool{sendMessageTool, reactTool, askUserTool, rememberTool, forgetTool, updateAgentTool, listAgentsTool, listModelsTool}
	tools = append(tools, taskTools()...)
	if sandboxes {
		tools = append(tools, runCommandTool, readFileTool, writeFileTool, listFilesTool)
	}
	if agent.IsAdmin {
		tools = append(tools, createAgentTool, archiveAgentTool, createGroupTool, updateGroupTool)
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
	case toolRemember:
		return "making a note"
	case toolForget:
		return "updating my notes"
	case toolListModels:
		return "checking models"
	case toolListAgents:
		return "checking the team"
	case toolCreateAgent:
		return "setting up an agent"
	case toolUpdateAgent:
		return "updating settings"
	case toolTaskCreate, toolTaskUpdate, toolTaskDelete:
		return "updating my schedule"
	case toolReadFile:
		return "reading a file"
	case toolWriteFile:
		return "writing a file"
	case toolListFiles:
		return "looking through files"
	case toolArchiveAgent:
		return "archiving an agent"
	case toolCreateGroup:
		return "setting up a group"
	case toolUpdateGroup:
		return "updating a group"
	default:
		return "using " + name
	}
}

// runTool executes a tool call and returns the result for the model, and whether it succeeded.
func (l *loop) runTool(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	args := []byte(call.Function.Arguments)
	if !slices.ContainsFunc(toolsFor(agent, l.m.sandboxesAvailable()), func(t model.Tool) bool { return t.Function.Name == call.Function.Name }) {
		if _, ok := l.connectorTool(ctx, agent, call.Function.Name); ok {
			return l.callConnector(ctx, agent, call.Function.Name, args)
		}
		return toolError("unknown tool %q", call.Function.Name), false
	}
	switch call.Function.Name {
	case toolSendMessage:
		return l.sendMessage(ctx, agent, args)
	case toolReact:
		return l.react(ctx, agent, args)
	case toolAskUser:
		return l.askUser(ctx, agent, args)
	case toolRemember:
		return l.remember(ctx, agent, args)
	case toolForget:
		var a struct {
			MemoryID string `json:"memory_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return toolError("invalid arguments: %v", err), false
		}
		if err := l.m.store.DeleteMemory(ctx, agent.ID, a.MemoryID); err != nil {
			return toolError("no memory with id %q", a.MemoryID), false
		}
		return toolOK(map[string]string{"forgot": a.MemoryID}), true
	case toolListAgents:
		agents, err := l.m.store.ListAgents(ctx)
		if err != nil {
			return toolError("couldn't list agents: %v", err), false
		}
		out := make([]map[string]any, 0, len(agents))
		for _, a := range agents {
			out = append(out, map[string]any{
				"agent_id": a.ID, "name": a.Name, "model": a.Model, "is_you": a.ID == agent.ID,
				"admin": a.IsAdmin, "about": truncate(strings.TrimSpace(a.Instructions), 200),
			})
		}
		return toolOK(map[string]any{"agents": out}), true
	case toolListModels:
		models, err := l.m.llm.Models(ctx)
		if err != nil {
			return toolError("couldn't load models: %v", err), false
		}
		return toolOK(map[string]any{"models": models, "your_model": agent.Model}), true
	case toolCreateAgent:
		return l.createAgent(ctx, agent, args)
	case toolUpdateAgent:
		return l.updateAgent(ctx, agent, args)
	case toolTaskCreate, toolTaskUpdate, toolTaskDelete:
		return l.runTaskTool(ctx, agent, call.Function.Name, args)
	case toolRunCommand, toolReadFile, toolWriteFile, toolListFiles:
		return l.runSandboxTool(ctx, agent, call.Function.Name, args)
	case toolArchiveAgent:
		return l.archiveAgent(ctx, agent, args)
	case toolCreateGroup:
		var a struct {
			Name     string   `json:"name"`
			AgentIDs []string `json:"agent_ids"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return toolError("invalid arguments: %v", err), false
		}
		chat, err := l.m.CreateGroup(ctx, a.Name, a.AgentIDs, &agent)
		if err != nil {
			return toolError("%v", err), false
		}
		return toolOK(map[string]any{"chat_id": chat.ID, "name": chat.Name, "members": len(chat.Members)}), true
	case toolUpdateGroup:
		var a struct {
			ChatID string   `json:"chat_id"`
			Name   *string  `json:"name"`
			Add    []string `json:"add_agent_ids"`
			Remove []string `json:"remove_agent_ids"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return toolError("invalid arguments: %v", err), false
		}
		chat, err := l.m.UpdateGroup(ctx, a.ChatID, a.Name, a.Add, a.Remove)
		if err != nil {
			return toolError("%v", err), false
		}
		return toolOK(map[string]any{"chat_id": chat.ID, "name": chat.Name, "members": len(chat.Members)}), true
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

const maxMemoryLength = 500

func (l *loop) remember(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		Text   string `json:"text"`
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	text := strings.TrimSpace(args.Text)
	if text == "" || utf8.RuneCountInString(text) > maxMemoryLength {
		return toolError("a memory must be 1–%d characters", maxMemoryLength), false
	}
	var source *string
	if args.ChatID != "" {
		if _, err := l.memberChat(ctx, agent, args.ChatID); err == nil {
			source = &args.ChatID
		}
	}
	mem, err := l.m.store.AddMemory(ctx, agent.ID, text, source)
	if err != nil {
		logger(agent.ID).Error("memory_save", "err", err)
		return toolError("failed to save the memory"), false
	}
	return toolOK(map[string]string{"memory_id": mem.ID}), true
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
	if !IsEmoji(emoji) {
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

// IsEmoji is a loose check that s is one short emoji (possibly with modifiers or joiners):
// no letters, digits, spaces or other ASCII.
func IsEmoji(s string) bool {
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
		SandboxWith  string `json:"sandbox_with"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	created, chatID, err := l.m.CreateAgent(ctx, NewAgent{
		SandboxWith:  args.SandboxWith,
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

func (l *loop) updateAgent(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		AgentID      string  `json:"agent_id"`
		Name         *string `json:"name"`
		Instructions *string `json:"instructions"`
		Model        *string `json:"model"`
		Language     *string `json:"language"`
		SandboxWith  string  `json:"sandbox_with"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if args.SandboxWith != "" && !agent.IsAdmin {
		return toolError("only admin agents can change who shares a computer"), false
	}
	target := agent.ID
	if args.AgentID != "" && args.AgentID != agent.ID {
		if !agent.IsAdmin {
			return toolError("you can only change your own settings"), false
		}
		target = args.AgentID
	}
	updated, err := l.m.UpdateAgent(ctx, target, store.AgentUpdate{
		Name: args.Name, Instructions: args.Instructions, Model: args.Model, Language: args.Language,
	})
	if err != nil {
		return toolError("%v", err), false
	}
	if args.SandboxWith != "" {
		if err := l.m.store.ShareSandbox(ctx, target, args.SandboxWith); err != nil {
			return toolError("couldn't share the computer: %v", err), false
		}
	}
	return toolOK(map[string]string{"agent_id": updated.ID, "name": updated.Name, "model": updated.Model, "language": updated.Language}), true
}

func (l *loop) archiveAgent(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if args.AgentID == agent.ID {
		return toolError("you can't archive yourself"), false
	}
	target, err := l.m.store.GetAgent(ctx, args.AgentID)
	if err != nil {
		return toolError("unknown agent_id %q", args.AgentID), false
	}
	if err := l.m.ArchiveAgent(ctx, target.ID); err != nil {
		return toolError("%v", err), false
	}
	return toolOK(map[string]string{"archived": target.Name}), true
}

func toolOK(v any) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": v})
	return string(b)
}

func toolError(format string, args ...any) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": fmt.Sprintf(format, args...)})
	return string(b)
}

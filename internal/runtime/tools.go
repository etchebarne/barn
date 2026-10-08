package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

const (
	toolSendMessage = "send_message"
	toolReact       = "react"
	toolDone        = "done"
	toolAskUser     = "ask_user"
	toolListModels  = "list_models"
	toolListAgents  = "list_agents"
	toolRemember    = "memory_save"
	toolForget      = "memory_forget"
	toolCreateAgent = "create_agent"
	toolUpdateAgent = "update_agent"
	toolDeleteAgent = "delete_agent"
	toolCreateGroup = "create_group"
	toolUpdateGroup = "update_group"
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
				"text": {"type": "string", "description": "The message, in Markdown. May be empty when you attach files."},
				"files": {"type": "array", "items": {"type": "string"}, "description": "Files from your computer to attach (up to 10, 25 MB each), e.g. /home/agent/report.pdf or /shared/chart.png. Images show in the chat."},
				"reply_to": {"type": "string", "description": "Optional: the message_id of an earlier message in this chat you're answering, shown quoted above yours. Use it when it wouldn't be clear otherwise (e.g. answering an older message in a busy group); not for every reply."}
			},
			"required": ["chat_id", "text"],
			"additionalProperties": false
		}`)

	doneTool = function(toolDone,
		"End your turn: call it once you've done everything this needs, or when there's nothing to "+
			"say or do. Don't repeat a message you already sent.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	reactTool = function(toolReact,
		"React to a message you've just received with an emoji, like people do in chat. Use it when "+
			"it doesn't need a written reply (thanks, an FYI, a done-update) or to acknowledge something "+
			"you're about to work on. Never to end a turn quietly: if there's nothing to say, just stop.",
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
		"List the agents in openbot (your teammates): their agent_id, name, what they do, and model.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	listModelsTool = function(toolListModels,
		"List the model ids available from the user's OpenCode Go subscription.",
		`{"type": "object", "properties": {}, "additionalProperties": false}`)

	updateAgentTool = function(toolUpdateAgent,
		"Change an agent's settings: rename it, rewrite its instructions or personality, or switch "+
			"its model or reply language. Leave agent_id out to change yourself. Only set the fields "+
			"that change, and only what the user asked for.",
		`{
			"type": "object",
			"properties": {
				"agent_id": {"type": "string", "description": "The agent to change; defaults to you."},
				"name": {"type": "string"},
				"instructions": {"type": "string", "description": "The full new instructions (they replace the old ones)."},
				"personality": {"type": "string", "description": "The full new personality (replaces the old one; empty for none): how the agent comes across, e.g. tone, voice, humour, emoji use. Not what it does (that's instructions) or facts (that's memory)."},
				"model": {"type": "string", "description": "Model id (from list_models)."},
				"language": {"type": "string", "description": "\"auto\" or a language name, e.g. \"Spanish\"."},
				"sandbox_with": {"type": "string", "description": "Admins only: an agent_id whose computer this agent should share from now on."}
			},
			"additionalProperties": false
		}`)

	deleteAgentTool = function(toolDeleteAgent,
		"Delete an agent for good: it stops, and its DM, memories and tasks are removed (its "+
			"messages in groups stay). Only when the user asks; it always needs their approval. "+
			"You can't delete yourself.",
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
				"instructions": {"type": "string", "description": "Detailed instructions: what it does, how, for whom, and the preferences the user mentioned. Stick to what the user told you: don't invent rules, protocols, message formats, schedules or processes they didn't ask for (suggest those to the user first), and don't describe setup like which apps are connected (the agent sees that itself)."},
				"personality": {"type": "string", "description": "Optional: how it comes across (tone, voice, humour, emoji use), in a sentence or two, when the user described one. Leave it out otherwise."},
				"model": {"type": "string", "description": "Model id (from list_models)."},
				"sandbox_with": {"type": "string", "description": "Optional agent_id: share that agent's computer instead of getting a new one (for agents that work on the same files)."},
				"connections": {"type": "array", "items": {"type": "string"}, "description": "Connections (by name or account_id, from \"All connections\" in your instructions) the agent needs for its job; it gets access right away."}
			},
			"required": ["name", "job", "instructions", "model"],
			"additionalProperties": false
		}`)
)

// toolsFor returns the tools an agent may use.
func toolsFor(agent store.Agent, sandboxes bool) []model.Tool {
	tools := []model.Tool{sendMessageTool, reactTool, doneTool, askUserTool, rememberTool, forgetTool, updateAgentTool, listAgentsTool, listModelsTool, allowWithoutAskingTool}
	tools = append(tools, taskTools()...)
	if sandboxes {
		tools = append(tools, runCommandTool, readFileTool, writeFileTool, listFilesTool, requestSecretTool)
	}
	if agent.IsAdmin {
		tools = append(tools, createAgentTool, deleteAgentTool, createGroupTool, updateGroupTool)
	}
	return tools
}

func toolLabel(name string) string {
	switch name {
	case toolSendMessage:
		return "writing a message"
	case toolReact:
		return "reacting"
	case toolDone:
		return "wrapping up"
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
	case toolDeleteAgent:
		return "deleting an agent"
	case toolCreateGroup:
		return "setting up a group"
	case toolUpdateGroup:
		return "updating a group"
	case toolConnectApp:
		return "setting up an app"
	case toolAllowWithoutAsking:
		return "asking to stop asking"
	default:
		return "using " + name
	}
}

// runTool executes a tool call and returns the result for the model, and whether it succeeded.
func (l *loop) runTool(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	args := []byte(call.Function.Arguments)
	if call.Function.Name == toolConnectApp && l.m.Connectors != nil {
		return l.connectApp(ctx, agent, args)
	}
	if !slices.ContainsFunc(toolsFor(agent, l.m.sandboxesAvailable()), func(t model.Tool) bool { return t.Function.Name == call.Function.Name }) {
		if _, ok := l.connectorTool(ctx, agent, call.Function.Name); ok {
			return l.callConnector(ctx, agent, call.Function.Name, args)
		}
		return toolError("unknown tool %q", call.Function.Name), false
	}
	switch call.Function.Name {
	case toolDone:
		return toolOK(map[string]string{"note": "Turn ended."}), true
	case toolRequestSecret:
		return l.requestSecret(ctx, agent, args)
	case toolAllowWithoutAsking:
		return l.allowWithoutAsking(ctx, agent, args)
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
	case toolDeleteAgent:
		return l.deleteAgent(ctx, agent, args)
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
		ChatID  string   `json:"chat_id"`
		Text    string   `json:"text"`
		Files   []string `json:"files"`
		ReplyTo string   `json:"reply_to"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if strings.TrimSpace(args.Text) == "" && len(args.Files) == 0 {
		return toolError("text is empty"), false
	}
	chat, err := l.memberChat(ctx, agent, args.ChatID)
	if err != nil {
		return toolError("%v", err), false
	}
	// Some models repeat their last message while trying to wrap up; post it once.
	key := chat.ID + "\x00" + strings.TrimSpace(args.Text) + "\x00" + strings.Join(args.Files, "\x00")
	if id, ok := l.sent[key]; ok {
		return toolOK(map[string]string{"message_id": id,
			"note": "You already sent exactly this in this turn, so it wasn't sent again. If you're finished, call done."}), true
	}
	attached, err := l.attachFiles(ctx, agent, chat.ID, args.Files)
	if err != nil {
		return toolError("couldn't attach: %v", err), false
	}
	msg, err := l.m.postMessage(ctx, store.NewMessage{ChatID: chat.ID, AuthorKind: "agent", AuthorAgentID: &agent.ID,
		Body: args.Text, AttachmentIDs: attached, ReplyTo: strings.TrimSpace(args.ReplyTo)})
	if errors.Is(err, store.ErrBadReply) {
		return toolError("reply_to %q isn't a message in this chat", args.ReplyTo), false
	}
	if err != nil {
		logger(agent.ID).Error("send_message", "err", err)
		return toolError("failed to send the message"), false
	}
	if l.sent != nil {
		l.sent[key] = msg.ID
	}
	return toolOK(map[string]string{"message_id": msg.ID}), true
}

// MaxMemoryLength is the longest a memory can be, in characters.
const MaxMemoryLength = 500

func (l *loop) remember(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		Text   string `json:"text"`
		ChatID string `json:"chat_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	text := strings.TrimSpace(args.Text)
	if text == "" || utf8.RuneCountInString(text) > MaxMemoryLength {
		return toolError("a memory must be 1–%d characters", MaxMemoryLength), false
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

// markFresh records that the agent was shown a message in this turn.
func (l *loop) markFresh(id string) {
	if l.fresh != nil {
		l.fresh[id] = true
	}
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
	if msg.AuthorKind == "agent" && msg.AuthorAgentID != nil && *msg.AuthorAgentID == agent.ID {
		return toolError("that's your own message; react to the message you're answering (its message_id is on its <message> tag)"), false
	}
	// Reacting is a reply to a message just received. Reacting to one already answered, later,
	// reads as a random reaction out of nowhere.
	if !l.fresh[msg.ID] {
		answered, err := l.m.store.PostedAfter(ctx, msg.ChatID, agent.ID, msg.ID)
		if err != nil {
			return toolError("failed to react"), false
		}
		if answered {
			return toolError("you already replied after that message, so a reaction now would look random; " +
				"react only to messages you've just received. If there's nothing to say, end your turn without text."), false
		}
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
		Name         string   `json:"name"`
		Job          string   `json:"job"`
		Instructions string   `json:"instructions"`
		Personality  string   `json:"personality"`
		Model        string   `json:"model"`
		SandboxWith  string   `json:"sandbox_with"`
		Connections  []string `json:"connections"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	created, chatID, err := l.m.CreateAgent(ctx, NewAgent{
		SandboxWith:  args.SandboxWith,
		Connections:  args.Connections,
		Name:         args.Name,
		Job:          args.Job,
		Instructions: args.Instructions,
		Personality:  args.Personality,
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
		Personality  *string `json:"personality"`
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
		Name: args.Name, Instructions: args.Instructions, Personality: args.Personality, Model: args.Model,
		Language: args.Language,
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

func (l *loop) deleteAgent(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var args struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if args.AgentID == agent.ID {
		return toolError("you can't delete yourself"), false
	}
	target, err := l.m.store.GetAgent(ctx, args.AgentID)
	if err != nil {
		return toolError("unknown agent_id %q", args.AgentID), false
	}
	if err := l.m.DeleteAgent(ctx, target.ID); err != nil {
		return toolError("%v", err), false
	}
	return toolOK(map[string]string{"deleted": target.Name}), true
}

func toolOK(v any) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "result": v})
	return string(b)
}

func toolError(format string, args ...any) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "error": fmt.Sprintf(format, args...)})
	return string(b)
}

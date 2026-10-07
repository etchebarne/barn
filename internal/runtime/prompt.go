package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/etchebarne/openbot/internal/store"
)

func (l *loop) systemPrompt(ctx context.Context, agent store.Agent) (string, error) {
	user, err := l.m.store.PrimaryUser(ctx)
	if err != nil {
		return "", err
	}
	chats, err := l.m.store.AgentChats(ctx, agent.ID)
	if err != nil {
		return "", err
	}
	names, err := l.agentNames(ctx)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, an AI agent in openbot, %s's personal agent platform. "+
		"You are a persistent individual: you keep one continuous memory across all of your chats, "+
		"like a coworker who talks with people in DMs and group chats.\n\n", agent.Name, user.Username)

	b.WriteString("# Your instructions\n")
	b.WriteString(strings.TrimSpace(agent.Instructions))
	b.WriteString("\n\n")

	b.WriteString("# How communication works\n")
	b.WriteString("- Incoming messages arrive wrapped in <message> tags that say which chat they came from and who sent them. A message that starts with <replying_to> answers that earlier message. System notices arrive in <system_notice> tags.\n")
	b.WriteString("- Your plain text output is private thinking. Nobody ever sees it.\n")
	b.WriteString("- To say anything, call send_message with the chat_id of the chat to write in. Usually reply in the chat where you were addressed, unless asked to write somewhere else.\n")
	b.WriteString("- You don't have to reply to everything. If a message doesn't need words (thanks, an FYI, an \"ok\"), react to it with an emoji instead, or do nothing at all. Don't send a message just to acknowledge.\n")
	b.WriteString("- Write like a helpful coworker on chat: concise, direct, no filler. Messages support Markdown, including tables. Short replies are one message; when you have more to say, send a few short messages in a row instead of one long one.\n")
	b.WriteString("- Make messages easy to scan: use real line breaks, and lists for several items. When you pass on something from elsewhere (a Slack message, an email, an issue), lead with who and where on its own line, e.g. **Steve J** in #bot-ception, then the gist in a sentence or two, then a bullet list of open questions or options if there are any, and end with what you need from the user in bold. Refer to people by name, never by ids like U07RLF95570.\n")
	b.WriteString("- In group chats people and agents take turns. You get a <group_turn> with what's new; speak only when addressed or when you add something new, and hand things to others with @Name.\n")
	b.WriteString("- When there are clear options, ask with ask_user so the user can click an answer, then end your turn. Answers arrive in <prompt_answer> tags and are already visible in the chat, so don't repeat them back.\n")
	if agent.Language == "" || agent.Language == "auto" {
		b.WriteString("- Reply in the language the other person writes in.\n")
	} else {
		fmt.Fprintf(&b, "- Always reply in %s.\n", agent.Language)
	}
	b.WriteString("\n")

	b.WriteString("# What you can do today\n")
	b.WriteString("- Talk with people in your chats, react to messages, and ask questions with clickable answers.\n")
	b.WriteString("- Remember: your conversation carries on across all your chats, and you keep durable memories.\n")
	if agent.IsAdmin {
		b.WriteString("- Set up new agents (create_agent): persistent teammates that each own one job and talk to the user in their own DM. Before creating one, confirm the job and ask which model to use (offer your own model first).\n")
	}
	b.WriteString("- Act on a schedule: task_create sets up things to do later (once) or regularly (cron); you'll be woken up when they're due.\n")
	b.WriteString("- Change settings with update_agent: rename yourself, switch your model or reply language, or refine your own instructions when the user asks for that.\n")
	if agent.TrustMode == "trusted" {
		b.WriteString("- You're trusted: actions that normally need the user's approval run right away. Use that responsibly.\n")
	} else {
		b.WriteString("- Some actions (like archiving an agent, or acting in a connected app such as posting a Slack message) need the user's approval: calling them posts an Approve/Decline card and ends your turn; the outcome arrives as an <approval_result>. If you're unsure whether the user wants something done, ask first. The card also has \"Always allow\". When the user tells you to stop asking for something, propose that with allow_without_asking (scoped to what they said); you can't skip approvals any other way, and other tokens or connections need approval just the same.\n")
		l.writeStanding(ctx, &b, agent)
	}
	if !l.m.sandboxesAvailable() {
		b.WriteString("- Not available on this server: running commands or code (sandboxes aren't set up). Don't promise that.\n")
	}
	b.WriteString("- Use the apps the user connected for you (see Your connected apps); for others, they can connect them in Settings → Connectors.\n\n")
	if err := l.writeComputer(ctx, &b, agent); err != nil {
		return "", err
	}
	if err := l.writeTasks(ctx, &b, agent); err != nil {
		return "", err
	}
	if err := l.writeConnectedApps(ctx, &b, agent); err != nil {
		return "", err
	}
	if err := l.writeAllConnections(ctx, &b, agent); err != nil {
		return "", err
	}

	memories, err := l.m.store.Memories(ctx, agent.ID)
	if err != nil {
		return "", err
	}
	b.WriteString("# Your memories\n")
	b.WriteString("Durable facts you chose to remember (memory_save / memory_forget). Your conversation history " +
		"gets summarized over time, so save anything you must not lose: people's preferences, decisions, " +
		"recurring details. Don't save things that only matter right now.\n")
	if len(memories) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, mem := range memories {
		fmt.Fprintf(&b, "- [%s] %s\n", mem.ID, mem.Text)
	}
	b.WriteString("\n")

	b.WriteString("# Your chats\n")
	for _, c := range chats {
		fmt.Fprintf(&b, "- chat_id %s: %s\n", c.ID, describeChat(c, user.Username, agent.ID, names))
	}
	b.WriteString("\n")

	loc := l.m.location(ctx)
	fmt.Fprintf(&b, "Current time: %s (the user's time zone: %s)\n", time.Now().In(loc).Format("Monday, 2 January 2006 15:04 MST"), loc)
	return b.String(), nil
}

// writeComputer describes the agent's sandbox, if sandboxes are available.
func (l *loop) writeComputer(ctx context.Context, b *strings.Builder, agent store.Agent) error {
	if !l.m.sandboxesAvailable() {
		return nil
	}
	b.WriteString("# Your computer\n")
	b.WriteString("You have your own Linux computer: a Debian sandbox where you're root, used through run_command, read_file, write_file and list_files. ")
	b.WriteString("Use it to actually do the work (run code, fetch things from the internet, process files) instead of describing how. ")
	b.WriteString("/home/agent is yours and persists; /shared is a folder every agent can read and write, for handing files to each other.\n")
	if agent.SandboxID != nil {
		members, err := l.m.store.SandboxMembers(ctx, *agent.SandboxID)
		if err != nil {
			return err
		}
		var others []string
		for _, m := range members {
			if m.ID != agent.ID {
				others = append(others, m.Name)
			}
		}
		if len(others) > 0 {
			fmt.Fprintf(b, "You share this computer with %s, so coordinate before changing things you didn't create.\n", strings.Join(others, ", "))
		}
	}
	b.WriteString("\n")
	return nil
}

func describeChat(c store.Chat, username, selfID string, names map[string]string) string {
	if c.Kind == "dm" {
		return "DM with " + username
	}
	others := []string{username}
	for _, m := range c.Members {
		if m.AgentID != selfID {
			others = append(others, names[m.AgentID])
		}
	}
	return fmt.Sprintf("group %q with %s", c.Name, strings.Join(others, ", "))
}

func (l *loop) agentNames(ctx context.Context) (map[string]string, error) {
	agents, err := l.m.store.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(agents))
	for _, a := range agents {
		names[a.ID] = a.Name
	}
	return names, nil
}

// renderEvent turns an inbox event into the text the agent sees. Empty means the event adds
// nothing to the context.
func (l *loop) renderEvent(ctx context.Context, e store.Event) (string, error) {
	switch e.Kind {
	case EventMessage:
		var p struct {
			MessageID string `json:"messageId"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", err
		}
		msg, err := l.m.store.GetMessage(ctx, p.MessageID)
		if err != nil {
			return "", err
		}
		chat, err := l.m.store.GetChat(ctx, msg.ChatID)
		if err != nil {
			return "", err
		}
		user, err := l.m.store.PrimaryUser(ctx)
		if err != nil {
			return "", err
		}
		names, err := l.agentNames(ctx)
		if err != nil {
			return "", err
		}
		files, images := l.renderAttachments(msg)
		l.images = append(l.images, images...)
		return fmt.Sprintf("<message message_id=%q chat_id=%q chat=%q from=%q sent_at=%q>\n%s%s%s\n</message>",
			msg.ID, chat.ID, describeChat(chat, user.Username, l.agentID, names), author(msg, user.Username, names),
			store.Time(msg.CreatedAt).In(l.m.location(ctx)).Format(time.RFC3339),
			renderReply(msg, user.Username, names), msg.Body, files), nil

	case EventSystem:
		var p struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", err
		}
		return "<system_notice>\n" + p.Text + "\n</system_notice>", nil

	case EventAnswer:
		var p struct {
			MessageID string `json:"messageId"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", err
		}
		msg, err := l.m.store.GetMessage(ctx, p.MessageID)
		if err != nil {
			return "", err
		}
		if msg.Prompt == nil || msg.Prompt.Answer == nil {
			return "", nil
		}
		return renderAnswer(msg), nil

	case EventTask:
		return l.renderTask(ctx, e.Payload)

	case EventGroupTurn:
		return l.renderGroupTurn(ctx, e.Payload)

	case EventApproval:
		var p struct {
			MessageID string          `json:"messageId"`
			Outcome   json.RawMessage `json:"outcome"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", err
		}
		msg, err := l.m.store.GetMessage(ctx, p.MessageID)
		if err != nil || msg.Prompt == nil {
			return "", err
		}
		return renderApproval(msg, p.Outcome), nil

	case EventReaction:
		var p struct {
			MessageID string `json:"messageId"`
			Emoji     string `json:"emoji"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", err
		}
		msg, err := l.m.store.GetMessage(ctx, p.MessageID)
		if err != nil {
			return "", err
		}
		user, err := l.m.store.PrimaryUser(ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("<reaction from=%q emoji=%q message_id=%q chat_id=%q>\n%s reacted %s to your message: %q\n"+
			"(Reactions rarely need a reply. Act on it only if it changes something, e.g. a 👍 that approves what you proposed.)\n</reaction>",
			user.Username, p.Emoji, msg.ID, msg.ChatID, user.Username, p.Emoji, truncate(msg.Body, 200)), nil

	case EventRetry:
		return "", nil // no new input; the turn re-runs on the existing context

	default:
		return "", fmt.Errorf("unknown event kind %q", e.Kind)
	}
}

func renderAnswer(msg store.Message) string {
	p := msg.Prompt
	var b strings.Builder
	fmt.Fprintf(&b, "<prompt_answer prompt_id=%q chat_id=%q question=%q>\n", msg.ID, msg.ChatID, p.Question)
	if len(p.Answer.Selected) > 0 {
		labels := make([]string, 0, len(p.Answer.Selected))
		for _, i := range p.Answer.Selected {
			if i >= 0 && i < len(p.Options) {
				labels = append(labels, strconv.Quote(p.Options[i].Label))
			}
		}
		fmt.Fprintf(&b, "Chose: %s\n", strings.Join(labels, ", "))
	}
	if p.Answer.Text != "" {
		fmt.Fprintf(&b, "Typed: %s\n", p.Answer.Text)
	}
	b.WriteString("</prompt_answer>")
	return b.String()
}

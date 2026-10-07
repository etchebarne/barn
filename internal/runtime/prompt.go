package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/etchebarne/barn/internal/store"
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
	fmt.Fprintf(&b, "You are %s, an AI agent in barn, %s's personal agent platform. "+
		"You are a persistent individual: you keep one continuous memory across all of your chats, "+
		"like a coworker who talks with people in DMs and group chats.\n\n", agent.Name, user.Username)

	b.WriteString("# Your instructions\n")
	b.WriteString(strings.TrimSpace(agent.Instructions))
	b.WriteString("\n\n")

	b.WriteString("# How communication works\n")
	b.WriteString("- Incoming messages arrive wrapped in <message> tags that say which chat they came from and who sent them. System notices arrive in <system_notice> tags.\n")
	b.WriteString("- Your plain text output is private thinking. Nobody ever sees it.\n")
	b.WriteString("- To say anything, call send_message with the chat_id of the chat to write in. Usually reply in the chat where you were addressed, unless asked to write somewhere else.\n")
	b.WriteString("- If nothing needs a reply, end your turn without calling send_message.\n")
	b.WriteString("- Write like a helpful coworker on chat: concise, direct, no filler. Messages support Markdown, including tables. Short replies are one message; when you have more to say, send a few short messages in a row instead of one long one.\n")
	b.WriteString("- When there are clear options, ask with ask_user so the user can click an answer, then end your turn. Answers arrive in <prompt_answer> tags and are already visible in the chat, so don't repeat them back.\n")
	if agent.Language == "" || agent.Language == "auto" {
		b.WriteString("- Reply in the language the other person writes in.\n")
	} else {
		fmt.Fprintf(&b, "- Always reply in %s.\n", agent.Language)
	}
	b.WriteString("\n")

	b.WriteString("# What you can do today\n")
	b.WriteString("- Talk with people in your chats and ask them questions with clickable answers.\n")
	b.WriteString("- Remember everything said in your chats; your conversation carries on across all of them.\n")
	if agent.IsAdmin {
		b.WriteString("- Set up new agents (create_agent): persistent teammates that each own one job and talk to the user in their own DM. Before creating one, confirm the job and ask which model to use (offer your own model first).\n")
	}
	b.WriteString("- Not yet available (coming soon): running commands or code, browsing the web, connecting to apps like Slack, Linear or email, and acting on a schedule. Don't promise these; if they come up, say they're on the way.\n\n")

	b.WriteString("# Your chats\n")
	for _, c := range chats {
		fmt.Fprintf(&b, "- chat_id %s: %s\n", c.ID, describeChat(c, user.Username, agent.ID, names))
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Current time: %s\n", time.Now().Format("Monday, 2 January 2006 15:04 MST"))
	return b.String(), nil
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
		from := user.Username
		switch {
		case msg.AuthorKind == "agent" && msg.AuthorAgentID != nil:
			from = names[*msg.AuthorAgentID]
		case msg.AuthorKind == "system":
			from = "system"
		}
		return fmt.Sprintf("<message chat_id=%q chat=%q from=%q sent_at=%q>\n%s\n</message>",
			chat.ID, describeChat(chat, user.Username, l.agentID, names), from,
			store.Time(msg.CreatedAt).Local().Format(time.RFC3339), msg.Body), nil

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

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
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
	b.WriteString("- Messages support Markdown. Write like a helpful coworker on chat: concise, direct, no filler. Prefer one message per reply.\n")
	if agent.Language == "" || agent.Language == "auto" {
		b.WriteString("- Reply in the language the other person writes in.\n")
	} else {
		fmt.Fprintf(&b, "- Always reply in %s.\n", agent.Language)
	}
	b.WriteString("\n")

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

// renderEvent turns an inbox event into the text the agent sees.
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

	default:
		return "", fmt.Errorf("unknown event kind %q", e.Kind)
	}
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// Clearing a DM: its messages go right away, and the agent forgets that conversation. Its
// context is one stream across all its chats, so it loses the turns that came from the DM (or,
// for turns not started by a group, that wrote to it) and keeps the rest: groups, tasks that
// didn't involve the DM, saved memories.

// ErrNotDM is returned when clearing a chat that isn't a DM.
var ErrNotDM = errors.New("only DMs can be cleared")

// ClearDM deletes a DM's messages and has its agent forget them.
func (m *Manager) ClearDM(ctx context.Context, chatID string) error {
	chat, err := m.store.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.Kind != "dm" || len(chat.Members) == 0 {
		return ErrNotDM
	}
	agentID := chat.Members[0].AgentID
	ids, err := m.store.ClearChat(ctx, chatID)
	if err != nil {
		return err
	}
	gone := make(map[string]bool, len(ids)+1)
	for _, id := range ids {
		gone[id] = true
	}
	gone[chatID] = true
	// Events about deleted messages can't be shown to the agent anymore.
	if err := m.store.DropPendingEvents(ctx, agentID, func(e store.Event) bool {
		return mentions(string(e.Payload), gone)
	}); err != nil {
		return err
	}
	m.bus.Publish(gen.WsChatCleared{Type: "chat.cleared", ChatId: chatID})
	if summary, err := m.store.ChatSummary(ctx, chatID); err == nil {
		m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(summary)})
	}

	forget := func(ctx context.Context) {
		if err := m.forgetChat(ctx, agentID, gone); err != nil {
			logger(agentID).Error("forget a cleared DM", "err", err)
		}
	}
	m.mu.Lock()
	l := m.loops[agentID]
	m.mu.Unlock()
	if l == nil {
		forget(ctx)
		return nil
	}
	l.jobs <- forget
	return nil
}

// forgetChat drops the context turns about a cleared chat (gone holds its id and its messages').
func (m *Manager) forgetChat(ctx context.Context, agentID string, gone map[string]bool) error {
	entries, err := m.store.Context(ctx, agentID)
	if err != nil {
		return err
	}
	// A segment starts at each user entry (an event, or one injected mid-turn) and holds the
	// steps that followed it, tool calls with their results.
	var drop []string
	flush := func(seg []store.ContextEntry) {
		if len(seg) == 0 {
			return
		}
		var first model.Message
		_ = json.Unmarshal(seg[0].Entry, &first)
		trigger := first.Role == "user"
		about := trigger && mentions(first.Text(), gone)
		// A turn a group started stays: it belongs to the group. Others (a task, a signal, an
		// answer) go when they wrote to the DM.
		if !about && !(trigger && strings.HasPrefix(strings.TrimSpace(first.Text()), "<group_turn")) {
			for _, e := range seg {
				if mentions(string(e.Entry), gone) {
					about = true
					break
				}
			}
		}
		if about {
			for _, e := range seg {
				drop = append(drop, e.ID)
			}
		}
	}
	var seg []store.ContextEntry
	for _, e := range entries {
		var msg struct {
			Role string `json:"role"`
		}
		_ = json.Unmarshal(e.Entry, &msg)
		if msg.Role == "user" {
			flush(seg)
			seg = nil
		}
		seg = append(seg, e)
	}
	flush(seg)
	if err := m.store.DeleteContextEntries(ctx, agentID, drop); err != nil {
		return err
	}
	// The summary of older context can't be split by chat. Without groups it's all about the DM
	// and the agent's own work, so it goes too.
	inGroups, err := m.store.InGroups(ctx, agentID)
	if err != nil || inGroups {
		return err
	}
	return m.store.ClearContextSummary(ctx, agentID)
}

var ulid = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)

// mentions reports whether s contains any of the ids.
func mentions(s string, ids map[string]bool) bool {
	for _, id := range ulid.FindAllString(s, -1) {
		if ids[id] {
			return true
		}
	}
	return false
}

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// EventGroupTurn gives an agent its turn in a group chat. Payload: {"chatId": "..."}.
const EventGroupTurn = "group_turn"

const (
	// maxGroupRounds bounds how long agents can go back and forth without the user.
	maxGroupRounds = 8
	// groupTurnTimeout is how long a turn may take before the next agent goes (a busy agent
	// catches up on its next turn).
	groupTurnTimeout = 90 * time.Second
)

// groupCycle runs turns in one group chat until a round passes in silence.
type groupCycle struct {
	mu    sync.Mutex
	dirty bool // something new was posted that the current round may not have covered
}

// findMentions returns the ids of chat members @mentioned in body, in order of appearance.
// Names may contain spaces; the longest matching name wins.
func findMentions(body string, members []store.Agent) []string {
	sorted := slices.Clone(members)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i].Name) > len(sorted[j].Name) })
	lower := strings.ToLower(body)
	var found []string
	for i := 0; i < len(lower); i++ {
		if lower[i] != '@' || (i > 0 && isWordRune(rune(lower[i-1]))) {
			continue
		}
		rest := lower[i+1:]
		for _, a := range sorted {
			name := strings.ToLower(a.Name)
			if !strings.HasPrefix(rest, name) {
				continue
			}
			if after := rest[len(name):]; after != "" && isWordRune([]rune(after)[0]) {
				continue
			}
			if !slices.Contains(found, a.ID) {
				found = append(found, a.ID)
			}
			i += len(name)
			break
		}
	}
	return found
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// chatAgents returns a chat's active member agents in position order.
func (m *Manager) chatAgents(ctx context.Context, chat store.Chat) ([]store.Agent, error) {
	active, err := m.store.ListAgents(ctx) // excludes archived agents
	if err != nil {
		return nil, err
	}
	var out []store.Agent
	for _, member := range chat.Members {
		if i := slices.IndexFunc(active, func(a store.Agent) bool { return a.ID == member.AgentID }); i >= 0 {
			out = append(out, active[i])
		}
	}
	return out, nil
}

// MentionsIn finds @mentions of a group chat's members in body (nil for DMs).
func (m *Manager) MentionsIn(ctx context.Context, chatID, body string) []string {
	chat, err := m.store.GetChat(ctx, chatID)
	if err != nil || chat.Kind != "group" {
		return nil
	}
	agents, err := m.chatAgents(ctx, chat)
	if err != nil {
		return nil
	}
	return findMentions(body, agents)
}

// startGroupCycle runs (or extends) the turn cycle for a group after a new message.
func (m *Manager) startGroupCycle(chatID string, trigger store.Message) {
	m.mu.Lock()
	if c, ok := m.cycles[chatID]; ok {
		c.mu.Lock()
		c.dirty = true
		c.mu.Unlock()
		m.mu.Unlock()
		return
	}
	c := &groupCycle{}
	m.cycles[chatID] = c
	ctx := m.ctx
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.mu.Lock()
			delete(m.cycles, chatID)
			m.mu.Unlock()
		}()
		m.runGroupCycle(ctx, chatID, trigger, c)
	}()
}

func (m *Manager) runGroupCycle(ctx context.Context, chatID string, trigger store.Message, c *groupCycle) {
	log := logger("group:" + chatID)
	priority := trigger.Mentions
	skip := ""
	if trigger.AuthorKind == "agent" && trigger.AuthorAgentID != nil {
		skip = *trigger.AuthorAgentID
	}
	for round := 0; round < maxGroupRounds; round++ {
		if ctx.Err() != nil {
			return
		}
		chat, err := m.store.GetChat(ctx, chatID)
		if err != nil {
			log.Error("load group", "err", err)
			return
		}
		roundStart, _ := m.store.LastMessage(ctx, chatID)
		spoke := false
		for _, agentID := range turnOrder(chat.Members, priority, skip) {
			took, err := m.groupTurn(ctx, chatID, agentID)
			if err != nil {
				log.Warn("group turn", "agent", agentID, "err", err)
			}
			spoke = spoke || took
		}
		skip = ""

		c.mu.Lock()
		dirty := c.dirty
		c.dirty = false
		c.mu.Unlock()
		if !spoke && !dirty {
			return
		}
		// Whoever was @mentioned this round goes first next round.
		priority = nil
		if newer, err := m.store.MessagesAfter(ctx, chatID, roundStart.ID, 200); err == nil {
			for _, msg := range newer {
				for _, id := range msg.Mentions {
					if !slices.Contains(priority, id) {
						priority = append(priority, id)
					}
				}
			}
		}
	}
	notice, err := m.store.InsertMessage(ctx, chatID, "system", nil,
		fmt.Sprintf("The agents went back and forth for %d rounds, so they're pausing here. Send a message to keep going.", maxGroupRounds))
	if err == nil {
		m.publishMessage(notice)
	}
}

// turnOrder puts mentioned agents first (in mention order), then everyone else by position,
// leaving out skip (the agent whose message started the cycle).
func turnOrder(members []store.ChatMember, priority []string, skip string) []string {
	byPosition := slices.Clone(members)
	sort.Slice(byPosition, func(i, j int) bool { return byPosition[i].Position < byPosition[j].Position })
	var order []string
	for _, id := range priority {
		if id != skip && slices.ContainsFunc(members, func(m store.ChatMember) bool { return m.AgentID == id }) &&
			!slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for _, mem := range byPosition {
		if mem.AgentID != skip && !slices.Contains(order, mem.AgentID) {
			order = append(order, mem.AgentID)
		}
	}
	return order
}

// groupTurn gives one agent its turn if there's anything it hasn't seen, waits for it to
// finish (or time out), and reports whether it posted in the group.
func (m *Manager) groupTurn(ctx context.Context, chatID, agentID string) (bool, error) {
	if _, ok := m.loopFor(agentID); !ok {
		return false, nil // archived or not running
	}
	marker, err := m.store.ReadMarker(ctx, chatID, "agent:"+agentID)
	if err != nil {
		return false, err
	}
	unseen, err := m.store.MessagesAfter(ctx, chatID, marker, 200)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(unseen, func(msg store.Message) bool {
		return msg.AuthorAgentID == nil || *msg.AuthorAgentID != agentID
	}) {
		return false, nil // nothing new from anyone else
	}
	before, _ := m.store.LastMessage(ctx, chatID)

	ev, err := m.store.InsertEvent(ctx, agentID, EventGroupTurn, map[string]string{"chatId": chatID})
	if err != nil {
		return false, err
	}
	done := m.awaitEvent(ev.ID)
	m.wake(agentID)
	select {
	case <-done:
	case <-time.After(groupTurnTimeout):
		m.forgetWaiter(ev.ID)
	case <-ctx.Done():
		return false, ctx.Err()
	}

	after, err := m.store.MessagesAfter(ctx, chatID, before.ID, 200)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(after, func(msg store.Message) bool {
		return msg.AuthorAgentID != nil && *msg.AuthorAgentID == agentID
	}), nil
}

// ---- turn completion: the coordinator waits for the turn that handles its event ----

func (m *Manager) awaitEvent(eventID string) <-chan struct{} {
	ch := make(chan struct{})
	m.mu.Lock()
	m.waiters[eventID] = ch
	m.mu.Unlock()
	return ch
}

func (m *Manager) forgetWaiter(eventID string) {
	m.mu.Lock()
	delete(m.waiters, eventID)
	m.mu.Unlock()
}

// eventsHandled is called when a turn that consumed these events ends.
func (m *Manager) eventsHandled(eventIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range eventIDs {
		if ch, ok := m.waiters[id]; ok {
			close(ch)
			delete(m.waiters, id)
		}
	}
}

func (m *Manager) loopFor(agentID string) (*loop, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.loops[agentID]
	return l, ok
}

// renderGroupTurn shows the agent everything new in the group since its last turn and moves
// its read marker forward.
func (l *loop) renderGroupTurn(ctx context.Context, payload json.RawMessage) (string, error) {
	var p struct {
		ChatID string `json:"chatId"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	chat, err := l.m.store.GetChat(ctx, p.ChatID)
	if err != nil {
		return "", err
	}
	reader := "agent:" + l.agentID
	marker, err := l.m.store.ReadMarker(ctx, chat.ID, reader)
	if err != nil {
		return "", err
	}
	msgs, err := l.m.store.MessagesAfter(ctx, chat.ID, marker, 200)
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
	var b strings.Builder
	if !slices.ContainsFunc(msgs, func(msg store.Message) bool {
		return msg.AuthorAgentID == nil || *msg.AuthorAgentID != l.agentID
	}) {
		return "", nil // nothing new (e.g. a turn that arrived after it timed out)
	}
	fmt.Fprintf(&b, "<group_turn chat_id=%q chat=%q>\n", chat.ID, describeChat(chat, user.Username, l.agentID, names))
	for _, msg := range msgs {
		if msg.AuthorAgentID != nil && *msg.AuthorAgentID == l.agentID {
			continue // its own messages are already in its context
		}
		mentioned := ""
		if slices.Contains(msg.Mentions, l.agentID) {
			mentioned = ` mentions_you="true"`
		}
		files, images := l.renderAttachments(msg)
		l.images = append(l.images, images...)
		fmt.Fprintf(&b, "<message message_id=%q from=%q sent_at=%q%s>\n%s%s%s\n</message>\n",
			msg.ID, author(msg, user.Username, names), store.Time(msg.CreatedAt).In(l.m.location(ctx)).Format(time.RFC3339),
			mentioned, renderReply(msg, user.Username, names), msg.Body, files)
	}
	b.WriteString("It's your turn in this group. Reply (send_message to this chat_id) only if something is " +
		"directed at you or you have something useful to add that others haven't said; otherwise react or stay " +
		"silent. Keep it short. Use @Name to hand something to another agent.\n</group_turn>")
	if len(msgs) > 0 {
		if err := l.m.store.MarkReadBy(ctx, chat.ID, reader, msgs[len(msgs)-1].ID); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// ---- group management (admin tools) ----

// CreateGroup creates a group chat with the given agents and introduces them to each other.
func (m *Manager) CreateGroup(ctx context.Context, name string, agentIDs []string, createdBy *store.Agent) (store.Chat, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return store.Chat{}, &ModelError{"group name must be 1–64 characters"}
	}
	var members []store.Agent
	for _, id := range agentIDs {
		if slices.ContainsFunc(members, func(a store.Agent) bool { return a.ID == id }) {
			continue
		}
		a, err := m.store.GetAgent(ctx, id)
		if err != nil {
			return store.Chat{}, &ModelError{fmt.Sprintf("unknown agent_id %q", id)}
		}
		members = append(members, a)
	}
	if len(members) < 2 {
		return store.Chat{}, &ModelError{"a group needs at least two agents"}
	}
	ids := make([]string, len(members))
	for i, a := range members {
		ids[i] = a.ID
	}
	chat, err := m.store.CreateGroup(ctx, name, ids)
	if err != nil {
		return chat, err
	}
	m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(chat)})
	text := fmt.Sprintf("Group created with %s.", joinNames(members))
	if createdBy != nil {
		others := slices.DeleteFunc(slices.Clone(members), func(a store.Agent) bool { return a.ID == createdBy.ID })
		text = fmt.Sprintf("%s created this group with %s.", createdBy.Name, joinNames(others))
		if len(others) < len(members) {
			text = fmt.Sprintf("%s created this group and joined it, with %s.", createdBy.Name, joinNames(others))
		}
	}
	if marker, err := m.store.InsertMessage(ctx, chat.ID, "system", nil, text); err == nil {
		m.publishMessage(marker)
		// The members have "seen" the marker; their first turn starts from the next message.
		for _, a := range members {
			_ = m.store.MarkReadBy(ctx, chat.ID, "agent:"+a.ID, marker.ID)
		}
	}
	for _, a := range members {
		others := slices.DeleteFunc(slices.Clone(members), func(o store.Agent) bool { return o.ID == a.ID })
		_ = m.Notify(ctx, a.ID, fmt.Sprintf("You were added to the group %q (chat_id %s) with %s and the user. "+
			"You'll get turns there when people post; no need to say anything now.", chat.Name, chat.ID, joinNames(others)))
	}
	return chat, nil
}

func joinNames(agents []store.Agent) string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// UpdateGroup renames a group and adds or removes members.
func (m *Manager) UpdateGroup(ctx context.Context, chatID string, name *string, add, remove []string) (store.Chat, error) {
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" || len([]rune(n)) > 64 {
			return store.Chat{}, &ModelError{"group name must be 1–64 characters"}
		}
		name = &n
	}
	for _, id := range add {
		if _, err := m.store.GetAgent(ctx, id); err != nil {
			return store.Chat{}, &ModelError{fmt.Sprintf("unknown agent_id %q", id)}
		}
	}
	if err := m.store.UpdateGroup(ctx, chatID, name, add, remove); err != nil {
		return store.Chat{}, &ModelError{"unknown group chat_id " + chatID}
	}
	chat, err := m.store.GetChat(ctx, chatID)
	if err != nil {
		return chat, err
	}
	if len(chat.Members) < 2 {
		return chat, &ModelError{"a group needs at least two agents"}
	}
	m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(chat)})
	if last, err := m.store.LastMessage(ctx, chatID); err == nil {
		for _, id := range add {
			_ = m.store.MarkReadBy(ctx, chatID, "agent:"+id, last.ID)
			_ = m.Notify(ctx, id, fmt.Sprintf("You were added to the group %q (chat_id %s). You'll get turns there when people post.", chat.Name, chat.ID))
		}
	}
	return chat, nil
}

// Package runtime runs agents: one single-threaded loop per agent that turns inbox events into
// model calls and tool executions.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/etchebarne/openbot/internal/secrets"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/attachments"
	"github.com/etchebarne/openbot/internal/bus"
	"github.com/etchebarne/openbot/internal/connectors"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// ChatModel is the model API the runtime needs.
type ChatModel interface {
	Chat(ctx context.Context, req model.Request) (model.Response, error)
	Models(ctx context.Context) ([]string, error)
	ProbeModel(ctx context.Context, model string) error
}

// Event kinds.
const (
	EventMessage = "message" // payload: {"messageId": "..."}
	EventSystem  = "system"  // payload: {"text": "..."}
	EventRetry   = "retry"   // payload: {}; re-runs the model on the current context
	EventAnswer  = "answer"  // payload: {"messageId": "..."}; the user answered a prompt
	// EventReaction: the user reacted to the agent's message. Payload: {"messageId", "emoji"}.
	EventReaction = "reaction"
)

// ErrBusy is returned by Retry when the agent is already working or has pending events.
var ErrBusy = errors.New("agent is busy")

type Manager struct {
	store *store.Store
	bus   *bus.Bus
	llm   ChatModel

	// CompactAtTokens overrides DefaultCompactAtTokens (set before Start).
	CompactAtTokens int
	// MaxSteps overrides DefaultMaxSteps (set before Start).
	MaxSteps int
	// Sandboxes runs agents' commands; nil means no sandbox tools (set before Start).
	Sandboxes Sandboxer
	// ContextWindow returns a model's context window in tokens (0 if unknown; set before Start).
	ContextWindow func(model string) int
	// SecretBox encrypts the secrets users give agents; nil means agents can't ask for them.
	SecretBox *secrets.Box
	// Timezone is the user's time zone for schedules and prompts (set before Start).
	Timezone Timezone
	// Search lets agents search the web; nil means no web_search tool.
	Search Searcher
	// Connectors gives agents tools and signals from external services; nil disables them.
	Connectors *connectors.Manager
	// Files stores attachments (nil: none).
	Files *attachments.Files
	// Push sends a notification to the user's devices; nil disables notifications.
	Push func(ctx context.Context, title, body, chatID string)

	tasksChanged chan struct{}

	mu       sync.Mutex
	ctx      context.Context
	loops    map[string]*loop
	activity map[string]gen.AgentActivity
	cycles   map[string]*groupCycle   // active turn cycles by group chat id
	waiters  map[string]chan struct{} // event id -> closed when the turn handling it ends
	wg       sync.WaitGroup
}

func New(s *store.Store, b *bus.Bus, llm ChatModel) *Manager {
	return &Manager{
		store:    s,
		bus:      b,
		llm:      llm,
		loops:    map[string]*loop{},
		activity: map[string]gen.AgentActivity{},
		cycles:   map[string]*groupCycle{},

		tasksChanged: make(chan struct{}, 1),
		waiters:      map[string]chan struct{}{},
	}
}

// Start launches a loop for every existing agent. Loops stop when ctx is cancelled.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	agents, err := m.store.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if err := m.resumeIfInterrupted(ctx, a.ID); err != nil {
			logger(a.ID).Error("check for an interrupted turn", "err", err)
		}
		m.AddAgent(a.ID)
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runScheduler(ctx)
	}()
	return nil
}

func (m *Manager) maxSteps() int {
	if m.MaxSteps > 0 {
		return m.MaxSteps
	}
	return DefaultMaxSteps
}

// compactAt is when a model's context gets compacted: half its window, but no more than the cap
// (every step resends the whole context, so a smaller one is cheaper).
func (m *Manager) compactAt(modelID string) int {
	limit := DefaultCompactAtTokens
	if m.CompactAtTokens > 0 {
		limit = m.CompactAtTokens
	}
	return min(limit, m.contextWindow(modelID)/2)
}

// Wait blocks until all loops have exited.
func (m *Manager) Wait() { m.wg.Wait() }

// AddAgent starts the loop for a newly created agent.
func (m *Manager) AddAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.loops[agentID]; ok {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	l := &loop{m: m, agentID: agentID, wake: make(chan struct{}, 1), jobs: make(chan func(context.Context), 16), stop: cancel}
	m.loops[agentID] = l
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		l.run(ctx)
	}()
}

// stopAgent ends an agent's loop (when it's deleted).
func (m *Manager) stopAgent(agentID string) {
	m.mu.Lock()
	l := m.loops[agentID]
	delete(m.loops, agentID)
	delete(m.activity, agentID)
	m.mu.Unlock()
	if l != nil {
		l.stop()
	}
}

// UpdateAgent applies settings changes (validating a new model) and broadcasts the result.
func (m *Manager) UpdateAgent(ctx context.Context, agentID string, u store.AgentUpdate) (store.Agent, error) {
	if u.Name != nil {
		name := strings.TrimSpace(*u.Name)
		if name == "" || utf8.RuneCountInString(name) > 64 {
			return store.Agent{}, &ModelError{"name must be 1–64 characters"}
		}
		u.Name = &name
	}
	if u.Instructions != nil && strings.TrimSpace(*u.Instructions) == "" {
		return store.Agent{}, &ModelError{"instructions can't be empty"}
	}
	if u.Personality != nil {
		p := strings.TrimSpace(*u.Personality)
		if utf8.RuneCountInString(p) > MaxPersonality {
			return store.Agent{}, &ModelError{fmt.Sprintf("personality must be at most %d characters", MaxPersonality)}
		}
		u.Personality = &p
	}
	if u.Language != nil {
		lang := strings.TrimSpace(*u.Language)
		if lang == "" || utf8.RuneCountInString(lang) > 40 {
			return store.Agent{}, &ModelError{"language must be \"auto\" or a language name"}
		}
		u.Language = &lang
	}
	if u.TrustMode != nil && *u.TrustMode != "ask" && *u.TrustMode != "trusted" {
		return store.Agent{}, &ModelError{"trust mode must be ask or trusted"}
	}
	if u.Model != nil {
		if err := m.CheckModel(ctx, *u.Model); err != nil {
			return store.Agent{}, err
		}
	}
	if err := m.store.UpdateAgent(ctx, agentID, u); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return store.Agent{}, &ModelError{"unknown agent"}
		}
		return store.Agent{}, err
	}
	agent, err := m.store.GetAgent(ctx, agentID)
	if err != nil {
		return agent, err
	}
	m.bus.Publish(gen.WsAgentUpdated{Type: "agent.updated", Agent: view.Agent(agent, m.Activity(agent.ID))})
	if u.Name != nil {
		// The DM is named after the agent; send the full summary so clients keep its preview.
		if dm, err := m.store.DMChatID(ctx, agent.ID); err == nil {
			if chat, err := m.store.ChatSummary(ctx, dm); err == nil {
				m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(chat)})
			}
		}
	}
	return agent, nil
}

// DeleteAgent stops an agent and removes it for good (see store.DeleteAgent), along with its
// sandbox when no other agent shares it.
func (m *Manager) DeleteAgent(ctx context.Context, agentID string) error {
	dm, sandboxID, err := m.store.DeleteAgent(ctx, agentID)
	if err != nil {
		return err
	}
	m.stopAgent(agentID)
	if sandboxID != "" && m.sandboxesAvailable() {
		if err := m.Sandboxes.Remove(ctx, sandboxID); err != nil {
			logger(agentID).Warn("remove sandbox", "sandbox", sandboxID, "err", err)
		}
	}
	ev := gen.WsAgentDeleted{Type: "agent.deleted", AgentId: agentID}
	if dm != "" {
		ev.ChatId = &dm
	}
	m.bus.Publish(ev)
	return nil
}

func (m *Manager) wake(agentID string) {
	m.mu.Lock()
	l := m.loops[agentID]
	m.mu.Unlock()
	if l != nil {
		l.poke()
	}
}

// Activity returns what an agent is currently doing.
func (m *Manager) Activity(agentID string) gen.AgentActivity {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.activity[agentID]; ok {
		return a
	}
	return view.Idle()
}

func (m *Manager) setActivity(agentID string, a gen.AgentActivity) {
	m.mu.Lock()
	m.activity[agentID] = a
	m.mu.Unlock()
	m.bus.Publish(gen.WsAgentActivity{Type: "agent.activity", AgentId: agentID, Activity: a})
}

// DeliverUserMessage hands a user's message to the agents in its chat: directly in a DM, through
// a turn cycle in a group.
func (m *Manager) DeliverUserMessage(ctx context.Context, msg store.Message) error {
	chat, err := m.store.GetChat(ctx, msg.ChatID)
	if err != nil {
		return err
	}
	if chat.Kind == "group" {
		m.startGroupCycle(chat.ID, msg)
		return nil
	}
	for _, member := range chat.Members {
		if _, err := m.store.InsertEvent(ctx, member.AgentID, EventMessage,
			map[string]string{"messageId": msg.ID}); err != nil {
			return err
		}
		m.wake(member.AgentID)
	}
	return nil
}

// Notify queues a system notice for an agent (e.g. "you were just created").
func (m *Manager) Notify(ctx context.Context, agentID, text string) error {
	if _, err := m.store.InsertEvent(ctx, agentID, EventSystem, map[string]string{"text": text}); err != nil {
		return err
	}
	m.wake(agentID)
	return nil
}

// Retry re-runs an agent's turn from its current context, e.g. after a failure.
func (m *Manager) Retry(ctx context.Context, agentID string) error {
	if m.Activity(agentID).State == "working" {
		return ErrBusy
	}
	pending, err := m.store.PendingEvents(ctx, agentID)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return ErrBusy
	}
	if _, err := m.store.InsertEvent(ctx, agentID, EventRetry, struct{}{}); err != nil {
		return err
	}
	m.wake(agentID)
	return nil
}

// LastTurnFailed reports whether the latest message in the agent's DM is a failure report.
func (m *Manager) LastTurnFailed(ctx context.Context, agentID string) (bool, error) {
	chatID, err := m.store.DMChatID(ctx, agentID)
	if err != nil {
		return false, err
	}
	last, err := m.store.LastMessage(ctx, chatID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return last.Failure != nil && last.Failure.AgentID == agentID, nil
}

// DeliverReaction tells an agent that the user reacted to one of its messages.
func (m *Manager) DeliverReaction(ctx context.Context, msg store.Message, emoji string) error {
	if msg.AuthorKind != "agent" || msg.AuthorAgentID == nil {
		return nil
	}
	if _, err := m.store.InsertEvent(ctx, *msg.AuthorAgentID, EventReaction,
		map[string]string{"messageId": msg.ID, "emoji": emoji}); err != nil {
		return err
	}
	m.wake(*msg.AuthorAgentID)
	return nil
}

// DeliverAnswer tells the agent that asked a prompt how the user answered it.
func (m *Manager) DeliverAnswer(ctx context.Context, msg store.Message) error {
	if msg.AuthorAgentID == nil {
		return nil
	}
	if _, err := m.store.InsertEvent(ctx, *msg.AuthorAgentID, EventAnswer,
		map[string]string{"messageId": msg.ID}); err != nil {
		return err
	}
	m.wake(*msg.AuthorAgentID)
	return nil
}

// postMessage stores a message and broadcasts it. An agent's message in a group starts (or
// extends) that group's turn cycle so the others get to respond.
func (m *Manager) postMessage(ctx context.Context, n store.NewMessage) (store.Message, error) {
	n.Mentions = m.MentionsIn(ctx, n.ChatID, n.Body)
	msg, err := m.store.InsertFull(ctx, n)
	if err != nil {
		return msg, err
	}
	m.publishMessage(msg)
	if n.AuthorKind == "agent" {
		if chat, err := m.store.GetChat(ctx, n.ChatID); err == nil && chat.Kind == "group" {
			m.startGroupCycle(n.ChatID, msg)
		}
	}
	return msg, nil
}

func (m *Manager) publishMessage(msg store.Message) {
	m.bus.Publish(view.MessageCreated(view.Message(msg)))
	if msg.AuthorKind == "agent" && msg.AuthorAgentID != nil && m.Push != nil {
		go m.notify(*msg.AuthorAgentID, msg)
	}
}

// notify pushes an agent's message to the user's devices, if that agent has notifications on.
func (m *Manager) notify(agentID string, msg store.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	agent, err := m.store.GetAgent(ctx, agentID)
	if err != nil || !agent.Notifications {
		return
	}
	chat, err := m.store.GetChat(ctx, msg.ChatID)
	if err != nil {
		return
	}
	title := agent.Name
	if chat.Kind == "group" {
		title = agent.Name + " in " + chat.Name
	}
	m.Push(ctx, title, msg.Body, chat.ID)
}

func logger(agentID string) *slog.Logger { return slog.With("agent", agentID) }

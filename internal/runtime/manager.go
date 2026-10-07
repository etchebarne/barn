// Package runtime runs agents: one single-threaded loop per agent that turns inbox events into
// model calls and tool executions.
package runtime

import (
	"context"
	"log/slog"
	"sync"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/bus"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

// ChatModel is the model API the runtime needs.
type ChatModel interface {
	Chat(ctx context.Context, req model.Request) (model.Response, error)
}

// Event kinds.
const (
	EventMessage = "message" // payload: {"messageId": "..."}
	EventSystem  = "system"  // payload: {"text": "..."}
)

type Manager struct {
	store *store.Store
	bus   *bus.Bus
	llm   ChatModel

	mu       sync.Mutex
	ctx      context.Context
	loops    map[string]*loop
	activity map[string]gen.AgentActivity
	wg       sync.WaitGroup
}

func New(s *store.Store, b *bus.Bus, llm ChatModel) *Manager {
	return &Manager{
		store:    s,
		bus:      b,
		llm:      llm,
		loops:    map[string]*loop{},
		activity: map[string]gen.AgentActivity{},
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
		m.AddAgent(a.ID)
	}
	return nil
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
	l := &loop{m: m, agentID: agentID, wake: make(chan struct{}, 1)}
	m.loops[agentID] = l
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		l.run(m.ctx)
	}()
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

// DeliverUserMessage queues a user's chat message for every agent in that chat.
func (m *Manager) DeliverUserMessage(ctx context.Context, msg store.Message) error {
	chat, err := m.store.GetChat(ctx, msg.ChatID)
	if err != nil {
		return err
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

// postMessage stores a message and broadcasts it.
func (m *Manager) postMessage(ctx context.Context, chatID, authorKind string, agentID *string, body string) (store.Message, error) {
	msg, err := m.store.InsertMessage(ctx, chatID, authorKind, agentID, body)
	if err != nil {
		return msg, err
	}
	m.bus.Publish(view.MessageCreated(view.Message(msg)))
	return msg, nil
}

func logger(agentID string) *slog.Logger { return slog.With("agent", agentID) }

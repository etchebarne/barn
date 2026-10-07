package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// MaxPersonality is the longest personality an agent can have, in characters.
const MaxPersonality = 2000

// NewAgent describes an agent to create.
type NewAgent struct {
	Name         string
	Job          string // one sentence; empty for the starter agent
	Instructions string
	Personality  string // how it comes across; may be empty
	Model        string
	IsAdmin      bool
	// CreatedBy is the agent that created this one (nil when created by the system).
	CreatedBy *store.Agent
	// Welcome overrides the system notice that starts the new agent's first turn.
	Welcome string
	// SandboxWith is an agent whose sandbox the new agent shares (empty: its own).
	SandboxWith string
	// Connections are connector accounts (ids or names) the new agent gets access to.
	Connections []string
}

// ModelError is a user-facing reason a model can't be used.
type ModelError struct{ Message string }

func (e *ModelError) Error() string { return e.Message }

// CheckModel verifies a model exists and works with the saved key. Problems the user can fix
// are returned as *ModelError.
func (m *Manager) CheckModel(ctx context.Context, id string) error {
	models, err := m.llm.Models(ctx)
	if err != nil {
		return fmt.Errorf("couldn't load models: %w", err)
	}
	if !slices.Contains(models, id) {
		return &ModelError{"unknown model " + id}
	}
	switch err := m.llm.ProbeModel(ctx, id); {
	case err == nil:
		return nil
	case errors.Is(err, model.ErrTrainsOnData):
		return &ModelError{id + " is blocked by your OpenCode workspace's Privacy settings " +
			"(its provider trains on request data). Pick another model, or allow these models in OpenCode."}
	case errors.Is(err, model.ErrInvalidKey), errors.Is(err, model.ErrNoKey):
		return &ModelError{"OpenCode Go rejected your API key. Replace it in Settings."}
	default:
		return fmt.Errorf("couldn't reach %s: %w", id, err)
	}
}

// CreateAgent creates an agent with its DM, starts it, and has it introduce itself.
func (m *Manager) CreateAgent(ctx context.Context, n NewAgent) (store.Agent, string, error) {
	n.Name = strings.TrimSpace(n.Name)
	n.Job = strings.TrimSpace(n.Job)
	if n.Name == "" || utf8.RuneCountInString(n.Name) > 64 {
		return store.Agent{}, "", &ModelError{"name must be 1–64 characters"}
	}
	if strings.TrimSpace(n.Instructions) == "" {
		return store.Agent{}, "", &ModelError{"instructions are required"}
	}
	n.Personality = strings.TrimSpace(n.Personality)
	if utf8.RuneCountInString(n.Personality) > MaxPersonality {
		return store.Agent{}, "", &ModelError{fmt.Sprintf("personality must be at most %d characters", MaxPersonality)}
	}
	if err := m.CheckModel(ctx, n.Model); err != nil {
		return store.Agent{}, "", err
	}
	accounts, err := m.resolveAccounts(ctx, n.Connections)
	if err != nil {
		return store.Agent{}, "", err
	}

	instructions := strings.TrimSpace(n.Instructions)
	if n.Job != "" {
		instructions = "Your job: " + n.Job + "\n\n" + instructions
	}
	agent, chatID, err := m.store.CreateAgentWithDM(ctx, store.Agent{
		Name:          n.Name,
		Instructions:  instructions,
		Personality:   n.Personality,
		Model:         n.Model,
		Language:      "auto",
		Notifications: true,
		TrustMode:     "ask",
		IsAdmin:       n.IsAdmin,
	})
	if err != nil {
		return agent, "", err
	}
	// Before the welcome, so the agent knows its apps from its first turn.
	for _, a := range accounts {
		if err := m.store.AddGrant(ctx, a.ID, agent.ID); err != nil {
			return agent, chatID, err
		}
	}
	if n.SandboxWith != "" {
		if err := m.store.ShareSandbox(ctx, agent.ID, n.SandboxWith); err != nil {
			logger(agent.ID).Warn("share sandbox", "with", n.SandboxWith, "err", err)
		}
	}
	m.AddAgent(agent.ID)
	m.bus.Publish(gen.WsAgentCreated{Type: "agent.created", Agent: view.Agent(agent, m.Activity(agent.ID))})
	if chat, err := m.store.ChatSummary(ctx, chatID); err == nil {
		m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(chat)})
	}

	if n.CreatedBy != nil {
		if dm, err := m.store.DMChatID(ctx, n.CreatedBy.ID); err == nil {
			marker, err := m.store.InsertEventMessage(ctx, dm, "Created "+agent.Name, store.MessageEvent{
				Kind: "agent_created", AgentID: agent.ID, ChatID: chatID,
			})
			if err == nil {
				m.publishMessage(marker)
			}
		}
	}

	welcome := n.Welcome
	if welcome == "" {
		creator := "the system"
		if n.CreatedBy != nil {
			creator = n.CreatedBy.Name
		}
		welcome = fmt.Sprintf("You were just created by %s. Your job: %s\n\n"+
			"Introduce yourself in your DM in two or three short messages: who you are and what you'll "+
			"take care of. Then ask the one or two things you need to know to get started, using "+
			"ask_user where there are clear options. Only promise what your tools let you do today. Before "+
			"saying an app or account is missing, check \"Your connected apps\" in your instructions: apps "+
			"listed there are already connected and yours to use.",
			creator, n.Job)
	}
	if err := m.Notify(ctx, agent.ID, welcome); err != nil {
		return agent, chatID, err
	}
	return agent, chatID, nil
}

// resolveAccounts finds connector accounts by id or name (any case).
func (m *Manager) resolveAccounts(ctx context.Context, refs []string) ([]store.ConnectorAccount, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	all, err := m.store.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.ConnectorAccount
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		i := slices.IndexFunc(all, func(a store.ConnectorAccount) bool { return a.ID == ref || strings.EqualFold(a.Name, ref) })
		if i < 0 {
			names := make([]string, len(all))
			for j, a := range all {
				names[j] = a.Name
			}
			if len(names) == 0 {
				return nil, &ModelError{"there are no connections yet; the user adds them in Settings → Connectors"}
			}
			return nil, &ModelError{fmt.Sprintf("no connection %q (connections: %s)", ref, strings.Join(names, ", "))}
		}
		out = append(out, all[i])
	}
	return out, nil
}

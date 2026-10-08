package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// A backup of agents' configuration: what makes each agent itself (settings, instructions,
// personality, memories, tasks, the connections it may use, standing approvals), so it can be
// restored here or on another server. Secrets, messages, files and skills aren't in it.
// Connections are referred to by name, since ids differ between servers.

// BackupFormat and BackupVersion identify a backup file.
const (
	BackupFormat  = "openbot-agents"
	BackupVersion = 1
)

// ExportAgents writes every agent's configuration.
func (m *Manager) ExportAgents(ctx context.Context) (gen.AgentsBackup, error) {
	out := gen.AgentsBackup{Format: BackupFormat, Version: BackupVersion, ExportedAt: time.Now().UTC(), Agents: []gen.AgentBackup{}}
	agents, err := m.store.ListAgents(ctx)
	if err != nil {
		return out, err
	}
	accounts, err := m.store.Accounts(ctx)
	if err != nil {
		return out, err
	}
	accountName := map[string]string{}
	for _, a := range accounts {
		accountName[a.ID] = a.Name
	}
	for _, a := range agents {
		b := gen.AgentBackup{Name: a.Name, Instructions: a.Instructions, Personality: a.Personality, Model: a.Model,
			Language: a.Language, Notifications: a.Notifications, TrustMode: gen.AgentBackupTrustMode(a.TrustMode),
			Admin: a.IsAdmin, Memories: []string{}, Tasks: []gen.TaskBackup{}, Connections: []gen.ConnectionRef{},
			Approvals: []gen.ApprovalBackup{}}
		memories, err := m.store.Memories(ctx, a.ID)
		if err != nil {
			return out, err
		}
		for _, mem := range memories {
			b.Memories = append(b.Memories, mem.Text)
		}
		tasks, err := m.store.Tasks(ctx, a.ID)
		if err != nil {
			return out, err
		}
		for _, t := range tasks {
			tb := gen.TaskBackup{Name: t.Name, Purpose: t.Purpose, Kind: gen.TaskBackupKind(t.Kind), Enabled: t.Enabled}
			switch t.Kind {
			case "cron":
				tb.Cron = &t.Cron
			case "once":
				at := store.Time(t.At)
				tb.At = &at
			case "signal":
				tb.Signal = &gen.TaskBackupSignal{Account: accountName[t.SignalAccountID], Type: t.SignalType, Match: t.SignalMatch}
				if tb.Signal.Match == nil {
					tb.Signal.Match = map[string]string{}
				}
			}
			if t.Check != "" {
				tb.Check = &t.Check
			}
			b.Tasks = append(b.Tasks, tb)
		}
		granted, err := m.store.AgentAccounts(ctx, a.ID)
		if err != nil {
			return out, err
		}
		for _, acct := range granted {
			b.Connections = append(b.Connections, gen.ConnectionRef{Name: acct.Name, Type: acct.Type})
		}
		rules, err := m.store.ApprovalRules(ctx, a.ID)
		if err != nil {
			return out, err
		}
		for _, r := range rules {
			ab := gen.ApprovalBackup{Action: r.Action, Label: r.Label, Match: r.Match}
			if ab.Match == nil {
				ab.Match = map[string]string{}
			}
			// connector:<account id>:<tool> → connector:<tool>, with the account by name.
			if rest, ok := strings.CutPrefix(r.Action, "connector:"); ok {
				id, tool, _ := strings.Cut(rest, ":")
				name := accountName[id]
				ab.Action, ab.Account = "connector:"+tool, &name
			}
			b.Approvals = append(b.Approvals, ab)
		}
		out.Agents = append(out.Agents, b)
	}
	return out, nil
}

// RestoreAgents creates the agents in a backup. Agents whose name is taken are skipped, so
// nothing is overwritten; what can't be restored (a connection this server doesn't have, a
// one-off task already past) is left out and noted.
func (m *Manager) RestoreAgents(ctx context.Context, b gen.AgentsBackup) (gen.RestoreResult, error) {
	out := gen.RestoreResult{Agents: []gen.RestoredAgent{}}
	if b.Format != BackupFormat {
		return out, &ModelError{"this isn't an openbot agents backup"}
	}
	if b.Version > BackupVersion {
		return out, &ModelError{"this backup is from a newer openbot; update this server first"}
	}
	existing, err := m.store.ListAgents(ctx)
	if err != nil {
		return out, err
	}
	taken := map[string]bool{}
	for _, a := range existing {
		taken[strings.ToLower(a.Name)] = true
	}
	accounts, err := m.store.Accounts(ctx)
	if err != nil {
		return out, err
	}
	accountByName := map[string]store.ConnectorAccount{}
	for _, a := range accounts {
		accountByName[strings.ToLower(a.Name)] = a
	}

	for _, ab := range b.Agents {
		res := gen.RestoredAgent{Name: strings.TrimSpace(ab.Name), Notes: []string{}}
		switch {
		case res.Name == "" || utf8.RuneCountInString(res.Name) > 64:
			res.Notes = append(res.Notes, "Skipped: its name must be 1–64 characters.")
		case strings.TrimSpace(ab.Instructions) == "":
			res.Notes = append(res.Notes, "Skipped: it has no instructions.")
		case taken[strings.ToLower(res.Name)]:
			res.Notes = append(res.Notes, "Skipped: an agent with this name already exists.")
		default:
			id, notes, err := m.restoreAgent(ctx, ab, accountByName)
			if err != nil {
				return out, err
			}
			taken[strings.ToLower(res.Name)] = true
			res.Restored, res.AgentId, res.Notes = true, &id, notes
		}
		out.Agents = append(out.Agents, res)
	}
	return out, nil
}

func (m *Manager) restoreAgent(ctx context.Context, ab gen.AgentBackup, accounts map[string]store.ConnectorAccount) (string, []string, error) {
	notes := []string{}
	trust := string(ab.TrustMode)
	if trust != "trusted" {
		trust = "ask"
	}
	language := strings.TrimSpace(ab.Language)
	if language == "" {
		language = "auto"
	}
	personality := strings.TrimSpace(ab.Personality)
	if utf8.RuneCountInString(personality) > MaxPersonality {
		personality = string([]rune(personality)[:MaxPersonality])
	}
	agent, chatID, err := m.store.CreateAgentWithDM(ctx, store.Agent{
		Name: strings.TrimSpace(ab.Name), Instructions: strings.TrimSpace(ab.Instructions), Personality: personality,
		Model: ab.Model, Language: language, Notifications: ab.Notifications, TrustMode: trust, IsAdmin: ab.Admin,
	})
	if err != nil {
		return "", nil, err
	}
	if err := m.CheckModel(ctx, ab.Model); err != nil {
		notes = append(notes, fmt.Sprintf("Its model %s can't be used here (%v); pick another in its settings.", ab.Model, err))
	}

	for _, text := range ab.Memories {
		text = strings.TrimSpace(text)
		if text == "" || utf8.RuneCountInString(text) > MaxMemoryLength {
			continue
		}
		if _, err := m.store.AddMemory(ctx, agent.ID, text, nil); err != nil {
			return agent.ID, notes, err
		}
	}

	for _, c := range ab.Connections {
		acct, ok := accounts[strings.ToLower(c.Name)]
		if !ok {
			notes = append(notes, fmt.Sprintf("No connection named %s here, so it doesn't have it.", c.Name))
			continue
		}
		if err := m.store.AddGrant(ctx, acct.ID, agent.ID); err != nil {
			return agent.ID, notes, err
		}
	}

	for _, tb := range ab.Tasks {
		change := TaskChange{Name: &tb.Name, Purpose: &tb.Purpose, Enabled: &tb.Enabled, Check: tb.Check}
		switch tb.Kind {
		case "cron":
			change.Cron = tb.Cron
		case "once":
			if tb.At == nil || !tb.At.After(time.Now()) {
				notes = append(notes, fmt.Sprintf("Left out the one-off task %q: its time has passed.", tb.Name))
				continue
			}
			at := tb.At.Format(time.RFC3339)
			change.At = &at
		case "signal":
			if tb.Signal == nil {
				continue
			}
			acct, ok := accounts[strings.ToLower(tb.Signal.Account)]
			if !ok {
				notes = append(notes, fmt.Sprintf("Left out the task %q: it runs on events from %s, which isn't connected here.", tb.Name, tb.Signal.Account))
				continue
			}
			change.Signal = &TaskSignal{AccountID: acct.ID, Type: tb.Signal.Type, Match: tb.Signal.Match}
		}
		if _, err := m.SaveTask(ctx, agent.ID, "", change); err != nil {
			notes = append(notes, fmt.Sprintf("Left out the task %q: %v.", tb.Name, err))
		}
	}

	for _, ap := range ab.Approvals {
		action := ap.Action
		if tool, ok := strings.CutPrefix(ap.Action, "connector:"); ok {
			name := ""
			if ap.Account != nil {
				name = *ap.Account
			}
			acct, found := accounts[strings.ToLower(name)]
			if !found {
				notes = append(notes, fmt.Sprintf("Left out the standing approval %q: %s isn't connected here.", ap.Label, name))
				continue
			}
			action = "connector:" + acct.ID + ":" + tool
		} else if !strings.HasPrefix(action, "tool:") {
			continue
		}
		if err := m.store.AllowAction(ctx, agent.ID, action, ap.Match, ap.Label); err != nil {
			return agent.ID, notes, err
		}
	}

	m.AddAgent(agent.ID)
	m.bus.Publish(gen.WsAgentCreated{Type: "agent.created", Agent: view.Agent(agent, m.Activity(agent.ID))})
	if chat, err := m.store.ChatSummary(ctx, chatID); err == nil {
		m.bus.Publish(gen.WsChatCreated{Type: "chat.created", Chat: view.Chat(chat)})
	}
	return agent.ID, notes, nil
}

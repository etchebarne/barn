package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/etchebarne/barn/internal/connectors"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// connectorTools returns the connector tools an agent may call (nil without connectors).
func (l *loop) connectorTools(ctx context.Context, agent store.Agent) []connectors.AgentTool {
	if l.m.Connectors == nil {
		return nil
	}
	tools, err := l.m.Connectors.ToolsFor(ctx, agent.ID)
	if err != nil {
		logger(agent.ID).Warn("load connector tools", "err", err)
		return nil
	}
	return tools
}

func (l *loop) connectorTool(ctx context.Context, agent store.Agent, name string) (connectors.AgentTool, bool) {
	if !strings.Contains(name, "__") {
		return connectors.AgentTool{}, false
	}
	tools := l.connectorTools(ctx, agent)
	i := slices.IndexFunc(tools, func(t connectors.AgentTool) bool { return t.Name == name })
	if i < 0 {
		return connectors.AgentTool{}, false
	}
	return tools[i], true
}

// activity is the "what I'm doing" line; connector tools read as "using <account>".
func (l *loop) activity(ctx context.Context, agent store.Agent, call model.ToolCall) string {
	if t, ok := l.connectorTool(ctx, agent, call.Function.Name); ok {
		if a, err := l.m.store.GetAccount(ctx, t.AccountID); err == nil {
			return "using " + a.Name
		}
	}
	return activityFor(call)
}

// asModelTools turns connector tools into model tool definitions, tagging each with its account.
func asModelTools(ctx context.Context, m *Manager, tools []connectors.AgentTool) []model.Tool {
	names := map[string]string{}
	out := make([]model.Tool, 0, len(tools))
	for _, t := range tools {
		label, ok := names[t.AccountID]
		if !ok {
			if a, err := m.store.GetAccount(ctx, t.AccountID); err == nil {
				label = a.Name
			}
			names[t.AccountID] = label
		}
		desc := "[" + label + "] " + t.Tool.Description
		if t.Tool.External {
			desc += " (Acts on the user's behalf: needs approval unless you're trusted.)"
		}
		out = append(out, model.Tool{Type: "function", Function: model.FunctionSpec{
			Name: t.Name, Description: desc, Parameters: t.Tool.Parameters,
		}})
	}
	return out
}

func (l *loop) callConnector(ctx context.Context, agent store.Agent, name string, args []byte) (string, bool) {
	result, err := l.m.Connectors.Call(ctx, agent.ID, name, json.RawMessage(args))
	var ue *connectors.UserError
	switch {
	case errors.As(err, &ue):
		return toolError("%s", ue.Message), false
	case err != nil:
		logger(agent.ID).Warn("connector call", "tool", name, "err", err)
		return toolError("%s failed: %v", name, err), false
	}
	b, err := json.Marshal(map[string]any{"ok": true, "result": result})
	if err != nil {
		return toolError("couldn't encode the result"), false
	}
	if len(b) > 60_000 {
		return toolError("the result is too large (%d bytes); ask for less (filters, fewer items)", len(b)), false
	}
	return string(b), true
}

// writeConnectedApps lists the agent's connected accounts and what can trigger signal tasks.
func (l *loop) writeConnectedApps(ctx context.Context, b *strings.Builder, agent store.Agent) error {
	b.WriteString("# Your connected apps\n")
	if l.m.Connectors == nil {
		b.WriteString("(none)\n\n")
		return nil
	}
	accts, types, err := l.m.Connectors.AgentAccounts(ctx, agent.ID)
	if err != nil {
		return err
	}
	if len(accts) == 0 {
		b.WriteString("None yet. When an app or MCP server would help, propose it with connect_app (the user " +
			"adds secrets on the card), or they can add one in Settings → Connectors.\n\n")
		return nil
	}
	b.WriteString("Use their tools (named <account>__<tool>); propose more with connect_app. To react to events, create a signal task: task_create " +
		"with on_signal {account, type, match}; match maps a field to text it must contain (case-insensitive).\n")
	for i, a := range accts {
		fmt.Fprintf(b, "- %s (%s, account_id %s)", a.Name, types[i].DisplayName(), a.ID)
		if _, expired := connectors.SignedIn(a); expired {
			b.WriteString(" [sign-in expired: its tools are unavailable until the user clicks Reconnect in Settings → Connectors]")
		}
		if sigs := types[i].SignalTypes(); len(sigs) > 0 {
			b.WriteString(". Signals: ")
			parts := make([]string, len(sigs))
			for j, s := range sigs {
				parts[j] = fmt.Sprintf("%s (%s; fields: %s)", s.Type, s.Description, strings.Join(s.Fields, ", "))
			}
			b.WriteString(strings.Join(parts, "; "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return nil
}

// writeAllConnections tells admin agents (who set up other agents) every connection the user
// has and who uses it, so they can give a new agent access and never claim an app is missing.
func (l *loop) writeAllConnections(ctx context.Context, b *strings.Builder, agent store.Agent) error {
	if !agent.IsAdmin {
		return nil
	}
	b.WriteString("# All connections\n")
	accounts, err := l.m.store.Accounts(ctx)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		b.WriteString("None yet. The user adds apps in Settings → Connectors.\n\n")
		return nil
	}
	b.WriteString("Every app the user has connected. When creating an agent, give it the ones its job needs " +
		"(create_agent connections). To change an existing agent's access, the user uses Settings → Connectors.\n")
	agents, _ := l.m.store.ListAgents(ctx)
	names := map[string]string{}
	for _, a := range agents {
		names[a.ID] = a.Name
	}
	for _, a := range accounts {
		typ := a.Type
		if t, ok := connectors.TypeByName(a.Type); ok {
			typ = t.DisplayName()
		}
		fmt.Fprintf(b, "- %s (%s, account_id %s): ", a.Name, typ, a.ID)
		ids, _ := l.m.store.Grants(ctx, a.ID)
		var users []string
		for _, id := range ids {
			if n, ok := names[id]; ok {
				users = append(users, n)
			}
		}
		if len(users) == 0 {
			b.WriteString("no agent uses it yet\n")
		} else {
			b.WriteString("used by " + strings.Join(users, ", ") + "\n")
		}
	}
	b.WriteString("\n")
	return nil
}

// DeliverSignal wakes agents whose signal tasks match an incoming event.
func (m *Manager) DeliverSignal(ctx context.Context, sig store.Signal, fields map[string]string) {
	tasks, err := m.store.SignalTasks(ctx, sig.AccountID)
	if err != nil {
		logger("signals").Error("load signal tasks", "err", err)
		return
	}
	for _, t := range tasks {
		if t.SignalType != sig.Type || !connectors.Match(fields, t.SignalMatch) {
			continue
		}
		if _, err := m.store.InsertEvent(ctx, t.AgentID, EventTask, map[string]any{
			"taskId": t.ID, "signalId": sig.ID, "scheduledFor": sig.ReceivedAt,
		}); err != nil {
			logger(t.AgentID).Error("deliver signal", "err", err)
			continue
		}
		fired := time.Now().UnixMilli()
		t.LastFiredAt = &fired
		_ = m.store.SaveTask(ctx, t)
		logger(t.AgentID).Info("signal task fired", "task", t.Name, "signal", sig.Type)
		m.wake(t.AgentID)
	}
}

// signalTaskFields validates on_signal for task_create/task_update.
func (l *loop) resolveSignal(ctx context.Context, agent store.Agent, raw json.RawMessage) (accountID, typ string, match map[string]string, err error) {
	var s struct {
		Account string            `json:"account"`
		Type    string            `json:"type"`
		Match   map[string]string `json:"match"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", "", nil, fmt.Errorf("on_signal: %v", err)
	}
	if l.m.Connectors == nil {
		return "", "", nil, fmt.Errorf("no apps are connected")
	}
	accts, types, err := l.m.Connectors.AgentAccounts(ctx, agent.ID)
	if err != nil {
		return "", "", nil, err
	}
	for i, a := range accts {
		if a.ID != s.Account && !strings.EqualFold(a.Name, s.Account) {
			continue
		}
		sigs := types[i].SignalTypes()
		j := slices.IndexFunc(sigs, func(t connectors.SignalType) bool { return t.Type == s.Type })
		if j < 0 {
			var valid []string
			for _, t := range sigs {
				valid = append(valid, t.Type)
			}
			return "", "", nil, fmt.Errorf("%s has no signal %q (available: %s)", a.Name, s.Type, strings.Join(valid, ", "))
		}
		return a.ID, s.Type, s.Match, nil
	}
	return "", "", nil, fmt.Errorf("no connected account %q (see Your connected apps)", s.Account)
}

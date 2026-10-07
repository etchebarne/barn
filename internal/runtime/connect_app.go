package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/etchebarne/barn/internal/connectors"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

const toolConnectApp = "connect_app"

// connectAppTool lets an agent propose a connection. It's built from the connector registry so
// the model sees which types exist and what each needs.
func connectAppTool() model.Tool {
	var b strings.Builder
	b.WriteString("Propose connecting an app or MCP server for the user. They get a card in your DM showing " +
		"what you filled in, type any secrets (tokens, API keys) there, and press Connect; barn checks it " +
		"works before saving. Never ask for secrets in chat. Your turn ends here; the outcome arrives as a " +
		"<connection_result>, and once connected its tools are yours to use. Types:\n")
	names := make([]string, 0, len(connectors.Registry))
	for _, t := range connectors.Registry {
		names = append(names, t.Name())
		fmt.Fprintf(&b, "- %s (%s): %s", t.Name(), t.DisplayName(), t.Description())
		if fields := t.ConfigFields(); len(fields) > 0 {
			parts := make([]string, len(fields))
			for i, f := range fields {
				parts[i] = fieldHint(f)
			}
			b.WriteString(" Config: " + strings.Join(parts, "; ") + ".")
		}
		if fields := t.CredentialFields(); len(fields) > 0 {
			labels := make([]string, len(fields))
			for i, f := range fields {
				labels[i] = f.Label
				if f.Optional {
					labels[i] += " (optional)"
				}
			}
			b.WriteString(" The user enters: " + strings.Join(labels, ", ") + ".")
		}
		b.WriteString("\n")
	}
	b.WriteString("Apps that connect by signing in (type mcp with this url; the user just clicks Sign in and " +
		"Allow, no token needed):")
	for _, app := range connectors.Catalog {
		fmt.Fprintf(&b, " %s %s;", app.Name, app.URL)
	}
	b.WriteString(" other MCP servers that use sign-in work the same way.\n")
	enum, _ := json.Marshal(names)
	return function(toolConnectApp, b.String(), `{
		"type": "object",
		"properties": {
			"type": {"type": "string", "enum": `+string(enum)+`},
			"name": {"type": "string", "description": "What to call it, e.g. \"Notion\" (defaults to the type's name)"},
			"config": {"type": "object", "additionalProperties": {"type": "string"}, "description": "The type's config fields (non-secret), e.g. {\"url\": \"https://mcp.example.com/mcp\"}"},
			"agent_ids": {"type": "array", "items": {"type": "string"}, "description": "Agents that get access (defaults to just you)"},
			"reason": {"type": "string", "description": "One short sentence for the user on why, shown on the card"}
		},
		"required": ["type"],
		"additionalProperties": false
	}`)
}

func fieldHint(f connectors.Field) string {
	s := f.Key
	if f.Optional {
		s += " (optional)"
	}
	if f.Help != "" {
		s += ": " + f.Help
	}
	return s
}

// connectApp posts a connect card in the agent's DM. It reports whether the card was posted,
// which ends the turn like asking a question does.
func (l *loop) connectApp(ctx context.Context, agent store.Agent, args []byte) (string, bool) {
	var a struct {
		Type     string            `json:"type"`
		Name     string            `json:"name"`
		Config   map[string]string `json:"config"`
		AgentIDs []string          `json:"agent_ids"`
		Reason   string            `json:"reason"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	t, ok := connectors.TypeByName(a.Type)
	if !ok {
		return toolError("unknown type %q", a.Type), false
	}
	config := map[string]string{}
	for k, v := range a.Config {
		i := slices.IndexFunc(t.ConfigFields(), func(f connectors.Field) bool { return f.Key == k })
		if i < 0 {
			return toolError("%s has no config field %q", t.DisplayName(), k), false
		}
		if v = strings.TrimSpace(v); v != "" {
			config[k] = v
		}
	}
	for _, f := range t.ConfigFields() {
		if !f.Optional && config[f.Key] == "" {
			return toolError("config.%s is required (%s)", f.Key, f.Help), false
		}
	}
	if u, ok := config["url"]; ok && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return toolError("config.url must start with https:// or http://"), false
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = t.DisplayName()
	}
	if len([]rune(name)) > 64 {
		return toolError("name must be at most 64 characters"), false
	}
	agentIDs, err := l.connectAgents(ctx, agent, a.AgentIDs)
	if err != nil {
		return toolError("%v", err), false
	}
	dm, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return toolError("couldn't find your DM to show the card"), false
	}
	// MCP servers that use sign-in get a Sign in button instead of key fields.
	signIn := t.Name() == "mcp" && connectors.UsesSignIn(ctx, config["url"])
	question := fmt.Sprintf("Connect %s (%s)?", name, t.DisplayName())
	if r := strings.TrimSpace(a.Reason); r != "" {
		question += "\n" + truncate(r, 300)
	}
	msg, err := l.m.store.InsertPrompt(ctx, dm, agent.ID, store.Prompt{
		Kind:     "connect",
		Question: question,
		Options:  []store.PromptOption{{Label: "Connect"}, {Label: "Decline"}},
		Connection: &store.PendingConnection{
			AgentID: agent.ID, Type: t.Name(), Name: name, Config: config, AgentIDs: agentIDs, SignIn: signIn,
		},
	})
	if err != nil {
		logger(agent.ID).Error("propose connection", "err", err)
		return toolError("failed to show the card"), false
	}
	l.m.publishMessage(msg)
	return toolOK(map[string]string{
		"status": "waiting_for_user",
		"note":   "The card is in your DM. Your turn ends here; the outcome arrives as a <connection_result>.",
	}), true
}

// connectAgents checks who gets access: just the agent by default. Only admins may grant others.
func (l *loop) connectAgents(ctx context.Context, agent store.Agent, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{agent.ID}, nil
	}
	agents, err := l.m.store.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		if !slices.ContainsFunc(agents, func(a store.Agent) bool { return a.ID == id }) {
			return nil, fmt.Errorf("unknown agent_id %q", id)
		}
		if id != agent.ID && !agent.IsAdmin {
			return nil, errors.New("you can only propose connections for yourself")
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

// ErrNotConnectPrompt is returned by Connect for a message that isn't a pending connect card.
var ErrNotConnectPrompt = errors.New("not a pending connect prompt")

// Connect accepts a proposed connection: it saves it with the user's credentials (checked with
// the app first), grants the proposed agents access, marks the card answered and tells the agent
// which tools it got. A rejected check returns the connector's error and leaves the card pending.
func (m *Manager) Connect(ctx context.Context, messageID, name string, creds map[string]string) (store.Message, error) {
	msg, err := m.store.GetMessage(ctx, messageID)
	if err != nil {
		return store.Message{}, err
	}
	p := msg.Prompt
	if p == nil || p.Kind != "connect" || p.Connection == nil {
		return store.Message{}, ErrNotConnectPrompt
	}
	if p.Status != "pending" {
		return store.Message{}, store.ErrPromptClosed
	}
	if m.Connectors == nil {
		return store.Message{}, errors.New("connectors aren't available")
	}
	c := *p.Connection
	if name = strings.TrimSpace(name); name == "" {
		name = c.Name
	}
	if creds == nil {
		creds = map[string]string{}
	}
	config := map[string]string{}
	for k, v := range c.Config {
		config[k] = v
	}
	acct, err := m.Connectors.Save(ctx, "", c.Type, name, creds, config)
	if err != nil {
		return store.Message{}, err
	}
	if err := m.store.SetGrants(ctx, acct.ID, c.AgentIDs); err != nil {
		_ = m.Connectors.Delete(ctx, acct.ID)
		return store.Message{}, err
	}
	return m.answerConnectCard(ctx, messageID, acct.ID, name)
}

// ConnectedBySignIn answers a connect card whose connection was made by signing in.
func (m *Manager) ConnectedBySignIn(ctx context.Context, messageID, accountID, name string) (store.Message, error) {
	return m.answerConnectCard(ctx, messageID, accountID, name)
}

// answerConnectCard marks a connect card connected and tells the agent which tools it got.
// If the card was answered meanwhile, the new connection is removed again.
func (m *Manager) answerConnectCard(ctx context.Context, messageID, accountID, name string) (store.Message, error) {
	var c store.PendingConnection
	msg, err := m.store.UpdatePrompt(ctx, messageID, func(p store.Prompt) (store.Prompt, error) {
		if p.Kind != "connect" || p.Connection == nil {
			return p, ErrNotConnectPrompt
		}
		p.Status, p.Answer = "answered", &store.PromptAnswer{Selected: []int{0}}
		p.Connection.AccountID, p.Connection.Name = accountID, name
		c = *p.Connection
		return p, nil
	})
	if err != nil {
		// Answered elsewhere in the meantime: don't leave a second connection behind.
		_ = m.Connectors.Delete(ctx, accountID)
		return store.Message{}, err
	}
	var tools []string
	if all, err := m.Connectors.ToolsFor(ctx, c.AgentID); err == nil {
		for _, t := range all {
			if t.AccountID == accountID {
				tools = append(tools, t.Name)
			}
		}
	}
	outcome := map[string]any{"connected": true, "account_id": accountID, "name": name}
	if slices.Contains(c.AgentIDs, c.AgentID) {
		outcome["your_new_tools"] = tools
	} else {
		outcome["note"] = "You proposed it for other agents, so you don't have access yourself."
	}
	if err := m.resolved(ctx, c.AgentID, msg.ID, outcome); err != nil {
		return msg, err
	}
	return msg, nil
}

// declineConnection tells the agent the user declined its proposed connection.
func (m *Manager) declineConnection(ctx context.Context, msg store.Message) error {
	return m.resolved(ctx, msg.Prompt.Connection.AgentID, msg.ID, map[string]any{"connected": false, "declined": true})
}

func (m *Manager) resolved(ctx context.Context, agentID, messageID string, outcome map[string]any) error {
	if _, err := m.store.InsertEvent(ctx, agentID, EventApproval, map[string]any{
		"messageId": messageID, "outcome": outcome,
	}); err != nil {
		return err
	}
	m.wake(agentID)
	return nil
}

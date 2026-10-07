package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// EventApproval delivers the outcome of an approval prompt. Payload: {"messageId": "..."}.
const EventApproval = "approval"

// gatedTools need the user's approval unless the agent is trusted.
var gatedTools = map[string]bool{
	toolArchiveAgent:       true,
	toolAllowWithoutAsking: true,
}

func (l *loop) needsApproval(ctx context.Context, agent store.Agent, call model.ToolCall) bool {
	if agent.TrustMode == "trusted" {
		return false
	}
	tool := call.Function.Name
	gated := gatedTools[tool]
	if !gated {
		t, ok := l.connectorTool(ctx, agent, tool)
		gated = ok && t.Tool.External
	}
	return gated && !l.alwaysAllowed(ctx, agent, call)
}

// requestApproval posts an Approve / Decline prompt in the agent's DM instead of running a
// gated tool call. The call runs when the user approves (see ResolveApproval).
func (l *loop) requestApproval(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	args := json.RawMessage(call.Function.Arguments)
	if !json.Valid(args) {
		return toolError("invalid arguments"), false
	}
	question, preview, err := l.describeAction(ctx, agent, call.Function.Name, args)
	if err != nil {
		return toolError("%v", err), false
	}
	dm, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return toolError("couldn't find your DM to ask for approval"), false
	}
	action := &store.PendingAction{AgentID: agent.ID, Tool: call.Function.Name, Args: args}
	options := []store.PromptOption{{Label: "Approve"}, {Label: "Decline"}}
	// Option 2 approves this and stops asking for the same action.
	if key, label, ok := l.actionKey(ctx, agent, call.Function.Name); ok {
		action.Rule = &store.StandingRule{Action: key, Label: label}
		options = append(options, store.PromptOption{Label: "Always allow"})
	}
	msg, err := l.m.store.InsertPrompt(ctx, dm, agent.ID, store.Prompt{
		Kind:     "approval",
		Question: question,
		Options:  options,
		Action:   action,
		Preview:  preview,
	})
	if err != nil {
		logger(agent.ID).Error("request approval", "err", err)
		return toolError("failed to ask for approval"), false
	}
	l.m.publishMessage(msg)
	return toolOK(map[string]string{
		"status": "waiting_for_approval",
		"note":   "This needs the user's approval, so I asked them in your DM. Your turn ends here; the outcome arrives as an <approval_result>.",
	}), true
}

// describeAction is the approval question (a line for notifications and the agent) and the
// preview the card shows.
func (l *loop) describeAction(ctx context.Context, agent store.Agent, tool string, args json.RawMessage) (string, *store.ActionPreview, error) {
	switch tool {
	case toolAllowWithoutAsking:
		return l.describeAllow(ctx, agent, args)
	case toolArchiveAgent:
		var a struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", nil, fmt.Errorf("invalid arguments: %w", err)
		}
		target, err := l.m.store.GetAgent(ctx, a.AgentID)
		if err != nil {
			return "", nil, fmt.Errorf("unknown agent_id %q", a.AgentID)
		}
		name, _ := json.Marshal(target.Name)
		return fmt.Sprintf("Archive %s? It stops working and its chat is hidden. Its history is kept.", target.Name),
			&store.ActionPreview{Title: "Archive agent", Verb: "Archive",
				Note:   "It stops working and its chat is hidden. Its history is kept.",
				Fields: []store.PreviewField{{Key: "agent", Label: "Agent", Value: name}}}, nil
	default:
		if t, ok := l.connectorTool(ctx, agent, tool); ok {
			p := &store.ActionPreview{Title: t.Tool.Title, Verb: t.Tool.Verb}
			if a, err := l.m.store.GetAccount(ctx, t.AccountID); err == nil {
				p.AppType, p.AppName = a.Type, a.Name
			}
			if p.Title == "" {
				p.Title = humanize(t.Tool.Name)
			}
			if p.Verb == "" {
				// Tool names are usually actions already: "Send message", "Create page".
				p.Verb = humanize(t.Tool.Name)
			}
			body := t.Tool.Body
			if body == "" {
				body = guessBody(args)
			}
			p.Fields, p.Body = previewFields(args, t.Tool.Labels, body)
			app := p.AppName
			if app == "" {
				app = "an app"
			}
			return fmt.Sprintf("Allow %s to use %s: %s?", agent.Name, app, p.Title), p, nil
		}
		p := &store.ActionPreview{Title: humanize(tool), Verb: "Approve"}
		p.Fields, p.Body = previewFields(args, nil, "")
		return fmt.Sprintf("Allow %s to %s?", agent.Name, strings.ToLower(p.Title)), p, nil
	}
}

// previewFields lists the arguments in the order the agent wrote them, pulling out the main
// content. Empty values are left out.
func previewFields(args json.RawMessage, labels map[string]string, body string) ([]store.PreviewField, *store.PreviewField) {
	fields := []store.PreviewField{}
	var main *store.PreviewField
	dec := json.NewDecoder(bytes.NewReader(args))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return fields, nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			break
		}
		if v := strings.TrimSpace(string(value)); v == `""` || v == "null" || v == "[]" || v == "{}" {
			continue
		}
		label := labels[key]
		if label == "" {
			label = humanize(key)
		}
		f := store.PreviewField{Key: key, Label: label, Value: value}
		if key == body && main == nil {
			main = &f
			continue
		}
		fields = append(fields, f)
	}
	return fields, main
}

// guessBody picks the argument that's most likely the main content when a tool doesn't say.
func guessBody(args json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	for _, key := range []string{"text", "body", "message", "content", "markdown", "description"} {
		var s string
		if json.Unmarshal(m[key], &s) == nil && s != "" {
			return key
		}
	}
	return ""
}

// humanize turns "create_entities" or "threadTs" into "Create entities" / "Thread ts".
func humanize(name string) string {
	var b strings.Builder
	for i, r := range name {
		switch {
		case r == '_' || r == '-' || r == '.':
			b.WriteRune(' ')
		case unicode.IsUpper(r) && i > 0:
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	s := strings.Join(strings.Fields(b.String()), " ")
	if s == "" {
		return name
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// ResolveApproval runs or skips the gated action behind an answered approval prompt, then tells
// the agent the outcome.
func (m *Manager) ResolveApproval(ctx context.Context, msg store.Message) error {
	p := msg.Prompt
	if p != nil && p.Kind == "connect" && p.Connection != nil && p.Answer != nil {
		// Connecting goes through Connect; an answer here is a decline.
		return m.declineConnection(ctx, msg)
	}
	if p == nil || p.Kind != "approval" || p.Action == nil || p.Answer == nil {
		return nil
	}
	choice := -1
	if len(p.Answer.Selected) == 1 {
		choice = p.Answer.Selected[0]
	}
	approved := choice == 0 || choice == 2
	outcome := map[string]any{"approved": approved}
	if choice == 2 && p.Action.Rule != nil {
		if err := m.store.AllowAction(ctx, p.Action.AgentID, p.Action.Rule.Action, nil, p.Action.Rule.Label); err != nil {
			return err
		}
		outcome["always_allowed"] = p.Action.Rule.Label + " (you won't need approval for this again)"
	}
	if approved {
		agent, err := m.store.GetAgent(ctx, p.Action.AgentID)
		if err != nil {
			return err
		}
		l := &loop{m: m, agentID: agent.ID}
		result, _ := l.runTool(ctx, agent, model.ToolCall{
			ID: "approved", Type: "function",
			Function: model.FunctionCall{Name: p.Action.Tool, Arguments: string(p.Action.Args)},
		})
		outcome["result"] = json.RawMessage(result)
	}
	if _, err := m.store.InsertEvent(ctx, p.Action.AgentID, EventApproval, map[string]any{
		"messageId": msg.ID, "outcome": outcome,
	}); err != nil {
		return err
	}
	m.wake(p.Action.AgentID)
	return nil
}

func renderApproval(msg store.Message, outcome json.RawMessage) string {
	tag := "approval_result"
	if msg.Prompt.Kind == "connect" {
		tag = "connection_result"
	}
	return fmt.Sprintf("<%s prompt_id=%q question=%q>\n%s\n</%s>",
		tag, msg.ID, msg.Prompt.Question, string(outcome), tag)
}

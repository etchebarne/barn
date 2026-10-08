package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

// Standing approvals: the user can let an agent take an action without asking each time, from
// an approval card's "Always allow" button or by telling the agent, which then proposes it with
// allow_without_asking (a card the user confirms). An agent can never grant itself one: text it
// reads elsewhere could pretend to be the user.

const toolAllowWithoutAsking = "allow_without_asking"

var allowWithoutAskingTool = function(toolAllowWithoutAsking,
	"When the user tells you to stop asking for approval for an action (e.g. \"don't ask me before "+
		"posting in #alerts\"), propose a standing approval with this. They confirm it with one tap on "+
		"a card in your DM; after that the action runs without asking. Scope it to what they said: "+
		"only_when limits it to calls with those argument values (e.g. {\"channel\": \"#alerts\"}). "+
		"Your turn ends here; the outcome arrives as an <approval_result>.",
	`{
		"type": "object",
		"properties": {
			"tool": {"type": "string", "description": "The action, by its tool name (e.g. slack__post_message)."},
			"only_when": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Argument values the call must have, e.g. {\"channel\": \"#bot-ception\"}. Omit to allow it with any arguments."}
		},
		"required": ["tool"],
		"additionalProperties": false
	}`)

// actionKey identifies a gated action across renames: openbot's own tools by name, app tools by
// connection id and tool. label is what the user sees.
func (l *loop) actionKey(ctx context.Context, agent store.Agent, tool string) (key, label string, ok bool) {
	if tool == toolAllowWithoutAsking {
		return "", "", false // never itself allowed without asking
	}
	if gatedTools[tool] {
		return "tool:" + tool, humanize(tool), true
	}
	t, found := l.connectorTool(ctx, agent, tool)
	if !found || !t.Tool.External {
		return "", "", false
	}
	label = t.Tool.Title
	if label == "" {
		label = humanize(t.Tool.Name)
	}
	if a, err := l.m.store.GetAccount(ctx, t.AccountID); err == nil {
		label += " · " + a.Name
	}
	return "connector:" + t.AccountID + ":" + t.Tool.Name, label, true
}

// alwaysAllowed reports whether a standing approval covers this call.
func (l *loop) alwaysAllowed(ctx context.Context, agent store.Agent, call model.ToolCall) bool {
	key, _, ok := l.actionKey(ctx, agent, call.Function.Name)
	if !ok {
		return false
	}
	allowed, err := l.m.store.ActionAllowed(ctx, agent.ID, key, json.RawMessage(call.Function.Arguments))
	if err != nil {
		logger(agent.ID).Warn("check standing approvals", "err", err)
	}
	return allowed
}

type allowArgs struct {
	Tool     string            `json:"tool"`
	OnlyWhen map[string]string `json:"only_when"`
}

// describeAllow validates an allow_without_asking call and builds its card.
func (l *loop) describeAllow(ctx context.Context, agent store.Agent, args json.RawMessage) (string, *store.ActionPreview, error) {
	var a allowArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", nil, fmt.Errorf("invalid arguments: %w", err)
	}
	_, label, ok := l.actionKey(ctx, agent, a.Tool)
	if !ok {
		return "", nil, fmt.Errorf("%q isn't an action that asks for approval", a.Tool)
	}
	if params := l.toolParams(ctx, agent, a.Tool); params != nil {
		for k := range a.OnlyWhen {
			if !slices.Contains(params, k) {
				return "", nil, fmt.Errorf("%s has no argument %q (it has: %s)", a.Tool, k, strings.Join(params, ", "))
			}
		}
	}
	action, _ := json.Marshal(label)
	fields := []store.PreviewField{{Key: "action", Label: "Action", Value: action}}
	cond := conditionText(a.OnlyWhen)
	if cond != "" {
		v, _ := json.Marshal(cond)
		fields = append(fields, store.PreviewField{Key: "only_when", Label: "Only when", Value: v})
	}
	question := fmt.Sprintf("Always allow %s: %s", agent.Name, label)
	if cond != "" {
		question += ", when " + cond
	}
	return question + "?", &store.ActionPreview{
		Title: "Always allow", Verb: "Always allow", Fields: fields,
		Note: fmt.Sprintf("%s won't ask before doing this again. You can take it back in %s's settings.", agent.Name, agent.Name),
	}, nil
}

func conditionText(match map[string]string) string {
	keys := make([]string, 0, len(match))
	for k := range match {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = humanize(k) + " is " + match[k]
	}
	return strings.Join(parts, " and ")
}

// toolParams lists a tool's argument names (nil when unknown).
func (l *loop) toolParams(ctx context.Context, agent store.Agent, tool string) []string {
	var schema json.RawMessage
	if t, ok := l.connectorTool(ctx, agent, tool); ok {
		schema = t.Tool.Parameters
	} else {
		for _, t := range toolsFor(agent, l.m.sandboxesAvailable(), l.m.searchAvailable()) {
			if t.Function.Name == tool {
				schema = t.Function.Parameters
			}
		}
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil || s.Properties == nil {
		return nil
	}
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// allowWithoutAsking saves the standing approval once the user confirmed the card.
func (l *loop) allowWithoutAsking(ctx context.Context, agent store.Agent, args []byte) (string, bool) {
	var a allowArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	key, label, ok := l.actionKey(ctx, agent, a.Tool)
	if !ok {
		return toolError("%q isn't an action that asks for approval", a.Tool), false
	}
	if cond := conditionText(a.OnlyWhen); cond != "" {
		label += ", when " + cond
	}
	if err := l.m.store.AllowAction(ctx, agent.ID, key, a.OnlyWhen, label); err != nil {
		return toolError("couldn't save it: %v", err), false
	}
	return toolOK(map[string]string{"allowed": label}), true
}

// writeStanding lists the agent's standing approvals in its system prompt.
func (l *loop) writeStanding(ctx context.Context, b *strings.Builder, agent store.Agent) {
	rules, err := l.m.store.ApprovalRules(ctx, agent.ID)
	if err != nil || len(rules) == 0 {
		return
	}
	b.WriteString("- The user always allows these without asking:")
	for _, r := range rules {
		b.WriteString(" " + r.Label + ";")
	}
	b.WriteString("\n")
}

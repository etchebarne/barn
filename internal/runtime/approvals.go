package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// EventApproval delivers the outcome of an approval prompt. Payload: {"messageId": "..."}.
const EventApproval = "approval"

// gatedTools need the user's approval unless the agent is trusted.
var gatedTools = map[string]bool{
	toolArchiveAgent: true,
}

func (l *loop) needsApproval(ctx context.Context, agent store.Agent, tool string) bool {
	if agent.TrustMode == "trusted" {
		return false
	}
	if gatedTools[tool] {
		return true
	}
	t, ok := l.connectorTool(ctx, agent, tool)
	return ok && t.Tool.External
}

// requestApproval posts an Approve / Decline prompt in the agent's DM instead of running a
// gated tool call. The call runs when the user approves (see ResolveApproval).
func (l *loop) requestApproval(ctx context.Context, agent store.Agent, call model.ToolCall) (string, bool) {
	args := json.RawMessage(call.Function.Arguments)
	if !json.Valid(args) {
		return toolError("invalid arguments"), false
	}
	question, err := l.describeAction(ctx, agent, call.Function.Name, args)
	if err != nil {
		return toolError("%v", err), false
	}
	dm, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return toolError("couldn't find your DM to ask for approval"), false
	}
	msg, err := l.m.store.InsertPrompt(ctx, dm, agent.ID, store.Prompt{
		Kind:     "approval",
		Question: question,
		Options:  []store.PromptOption{{Label: "Approve"}, {Label: "Decline"}},
		Action:   &store.PendingAction{AgentID: agent.ID, Tool: call.Function.Name, Args: args},
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

// describeAction is the approval question shown to the user.
func (l *loop) describeAction(ctx context.Context, agent store.Agent, tool string, args json.RawMessage) (string, error) {
	switch tool {
	case toolArchiveAgent:
		var a struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		target, err := l.m.store.GetAgent(ctx, a.AgentID)
		if err != nil {
			return "", fmt.Errorf("unknown agent_id %q", a.AgentID)
		}
		return fmt.Sprintf("Archive %s? It stops working and its chat is hidden. Its history is kept.", target.Name), nil
	default:
		if t, ok := l.connectorTool(ctx, agent, tool); ok {
			account := "an app"
			if a, err := l.m.store.GetAccount(ctx, t.AccountID); err == nil {
				account = a.Name
			}
			return fmt.Sprintf("Allow %s to use %s: %s?%s", agent.Name, account, t.Tool.Name, describeArgs(args)), nil
		}
		return fmt.Sprintf("Allow %s to run %s with %s?", agent.Name, tool, truncate(string(args), 300)), nil
	}
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
	approved := len(p.Answer.Selected) == 1 && p.Answer.Selected[0] == 0
	outcome := map[string]any{"approved": approved}
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

// describeArgs lists tool arguments as "key: value" lines for an approval question.
func describeArgs(args json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil || len(m) == 0 {
		if len(args) == 0 || string(args) == "{}" {
			return ""
		}
		return "\n" + truncate(string(args), 400)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		v := string(m[k])
		var s string
		if json.Unmarshal(m[k], &s) == nil {
			v = s
		}
		fmt.Fprintf(&b, "\n%s: %s", k, truncate(v, 200))
	}
	return truncate(b.String(), 800)
}

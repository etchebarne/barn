package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// Tasks and app events run separately from the agent's main conversation, in a slim context:
// the same system prompt and summary (so the provider's cache of them still applies), a digest
// of the latest DM messages, and the event. Nothing of the run goes into the main conversation,
// except a short report of what the agent did that others can see (it can then follow up when
// the user replies). A poll that finds nothing leaves no trace and costs a fraction of a turn.

// EventTaskReport records what a task run did. It doesn't start a turn: it joins the main
// conversation with the next real event. Payload: {"text": "..."}.
const EventTaskReport = "task_report"

// digestMessages is how many recent DM messages a task run sees.
const digestMessages = 10

const taskRunNote = "This is a separate run for a task or app event, outside your main conversation " +
	"(you see its summary and your latest DM messages above, not the full history). Do what it " +
	"needs with your tools. If there's nothing worth telling anyone, call done without sending " +
	"anything. Messages you send and actions you take here are noted in your main conversation."

// runTasks runs task events: each task's events (a burst of app events, say) in one run.
func (l *loop) runTasks(ctx context.Context, events []store.Event) {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	// Consumed up front: a run that's cut short (a restart) isn't repeated with its side effects.
	if err := l.m.store.ConsumeEvents(ctx, l.agentID, ids, nil); err != nil {
		logger(l.agentID).Error("consume task events", "err", err)
		return
	}
	var order []string
	byTask := map[string][]store.Event{}
	for _, e := range events {
		var p struct {
			TaskID string `json:"taskId"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		if _, ok := byTask[p.TaskID]; !ok {
			order = append(order, p.TaskID)
		}
		byTask[p.TaskID] = append(byTask[p.TaskID], e)
	}
	for _, id := range order {
		if ctx.Err() != nil {
			return
		}
		if err := l.runTask(ctx, id, byTask[id]); err != nil && ctx.Err() == nil {
			logger(l.agentID).Warn("task run failed", "task", id, "err", err)
		}
	}
}

func (l *loop) runTask(ctx context.Context, taskID string, events []store.Event) error {
	agent, err := l.m.store.GetAgent(ctx, l.agentID)
	if err != nil {
		return err
	}
	task, err := l.m.store.GetTask(ctx, taskID)
	if err != nil {
		return nil // deleted since it fired
	}
	var parts []string
	for _, e := range events {
		text, err := l.renderTask(ctx, e.Payload)
		if err != nil {
			return err
		}
		if text != "" {
			parts = append(parts, l.stamp(ctx, text, e.CreatedAt))
		}
	}
	if len(parts) == 0 {
		return nil
	}

	l.m.setActivity(agent.ID, view.Working("running "+task.Name))
	defer l.m.setActivity(agent.ID, view.Idle())
	l.fresh, l.sent, l.aside, l.turnChars = map[string]bool{}, map[string]string{}, nil, 0

	msgs, err := l.head(ctx, agent)
	if err != nil {
		return err
	}
	digest, err := l.dmDigest(ctx, agent)
	if err != nil {
		return err
	}
	msgs = append(msgs, model.Text("user", digest+"\n\n"+strings.Join(parts, "\n\n")+"\n\n"+taskRunNote))
	tools := l.tools(ctx, agent)

	var report []string
	stopped := ""
	for step := 0; ; step++ {
		if step == l.m.maxSteps() {
			stopped = fmt.Sprintf("Stopped after %d steps without finishing.", step)
			break
		}
		resp, err := l.m.chat(ctx, agent.ID, "task", model.Request{
			Session: "openbot-agent-" + agent.ID, Model: agent.Model, Messages: msgs, Tools: tools,
		})
		if err != nil {
			return err
		}
		reply := resp.Message
		reply.Role = "assistant"
		msgs = append(msgs, reply)
		if len(reply.ToolCalls) == 0 {
			break
		}
		asked := false
		for _, call := range reply.ToolCalls {
			call = unwrapAppCall(call)
			call.Function.Arguments = l.fixProse(ctx, agent, call)
			l.m.setActivity(agent.ID, view.Working(l.activity(ctx, agent, call)))
			var result string
			var ok bool
			if l.needsApproval(ctx, agent, call) {
				result, ok = l.requestApproval(ctx, agent, call)
				asked = asked || ok
				if ok {
					report = append(report, "Asked for approval to use "+call.Function.Name+"; the outcome arrives here.")
				}
			} else {
				result, ok = l.runTool(ctx, agent, call)
				result = l.limitResult(ctx, agent, call, l.maskSecrets(ctx, agent.ID, result))
				if ok {
					if line := l.reportLine(ctx, agent, call); line != "" {
						report = append(report, line)
					}
				}
			}
			if ok && (call.Function.Name == toolAskUser || call.Function.Name == toolConnectApp || call.Function.Name == toolRequestSecret) {
				asked = true
			}
			msgs = append(msgs, model.Message{Role: "tool", Content: &result, ToolCallID: call.ID})
		}
		if asked || slices.ContainsFunc(reply.ToolCalls, func(c model.ToolCall) bool { return c.Function.Name == toolDone }) {
			break
		}
	}
	if stopped != "" {
		report = append(report, stopped)
	}
	if len(report) == 0 {
		return nil // a quiet run leaves no trace
	}
	text := fmt.Sprintf("<task_report task_id=%q name=%q>\nYou ran this in a separate run (not shown here). What you did:\n- %s\n</task_report>",
		task.ID, task.Name, strings.Join(report, "\n- "))
	_, err = l.m.store.InsertEvent(ctx, agent.ID, EventTaskReport, map[string]string{"text": text})
	return err
}

// dmDigest is the latest messages in the agent's DM, as plain lines.
func (l *loop) dmDigest(ctx context.Context, agent store.Agent) (string, error) {
	dm, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return "", err
	}
	msgs, _, err := l.m.store.ListMessages(ctx, dm, "", digestMessages)
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
	loc := l.m.location(ctx)
	var b strings.Builder
	fmt.Fprintf(&b, "<recent_dm chat_id=%q note=\"your latest messages with %s, for context\">\n", dm, user.Username)
	if len(msgs) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, m := range msgs {
		body := strings.Join(strings.Fields(m.Body), " ")
		if len(m.Attachments) > 0 {
			body += fmt.Sprintf(" [%d file(s)]", len(m.Attachments))
		}
		fmt.Fprintf(&b, "%s (%s): %s\n", author(m, user.Username, names),
			store.Time(m.CreatedAt).In(loc).Format("Mon 15:04"), truncate(body, 400))
	}
	b.WriteString("</recent_dm>")
	return b.String(), nil
}

// reportLine describes a tool call others can see (a message, an action in an app, a change),
// for the task report. Reading and looking things up aren't reported.
func (l *loop) reportLine(ctx context.Context, agent store.Agent, call model.ToolCall) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	str := func(k string) string { s, _ := args[k].(string); return s }
	switch call.Function.Name {
	case toolSendMessage:
		where := str("chat_id")
		if chat, err := l.m.store.GetChat(ctx, where); err == nil {
			where = chat.Name
		}
		return fmt.Sprintf("Sent in %s: %q", where, truncate(strings.Join(strings.Fields(str("text")), " "), 300))
	case toolReact:
		return "Reacted " + str("emoji") + " to message " + str("message_id")
	case toolAskUser:
		return fmt.Sprintf("Asked the user: %q (the answer arrives here)", truncate(str("question"), 200))
	case toolRequestSecret:
		return "Asked the user for the secret " + str("name") + " (the outcome arrives here)"
	case toolConnectApp:
		return "Proposed connecting an app (the outcome arrives here)"
	case toolRemember:
		return fmt.Sprintf("Saved a memory: %q", truncate(str("text"), 200))
	case toolForget:
		return "Forgot memory " + str("memory_id")
	case toolTaskCreate, toolTaskUpdate, toolTaskDelete, toolUpdateAgent, toolCreateAgent, toolDeleteAgent,
		toolCreateGroup, toolUpdateGroup, toolAllowWithoutAsking, toolWriteFile:
		return "Called " + call.Function.Name + " " + truncate(call.Function.Arguments, 300)
	}
	if t, ok := l.connectorTool(ctx, agent, call.Function.Name); ok && t.Tool.External {
		return "Used " + call.Function.Name + " " + truncate(call.Function.Arguments, 300)
	}
	return ""
}

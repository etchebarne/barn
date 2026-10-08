package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// DefaultMaxSteps bounds model calls in a single turn. Work on an agent's computer easily takes
// dozens of commands, so it's generous; it's there to stop a runaway turn.
const DefaultMaxSteps = 200

// wrapUpText asks for a progress update when a turn reaches the step limit.
const wrapUpText = "[system] You've taken %d steps in this turn, the most one turn allows, so it pauses here. " +
	"Don't start anything new: send the user a short progress update (what's done, what's left, what " +
	"you'd do next). They can tell you to continue."

var errTooManySteps = errors.New("too many steps in one turn")

// nudgeText doesn't suggest reacting: on turns no message started (a task, a signal) there's
// no message to react to, and models then reacted to an old one to "end quietly".
const nudgeText = "[system] Your last reply was plain text, which nobody can see. " +
	"If you meant to tell someone something, call send_message. If there's nothing to say, " +
	"that's fine: call done."

type loop struct {
	m       *Manager
	agentID string
	wake    chan struct{}
	jobs    chan func(context.Context) // run between turns, never during one
	stop    context.CancelFunc
	handled []string          // events consumed by the current turn
	fresh   map[string]bool   // messages the agent was shown in the current turn
	sent    map[string]string // chat + text + files → message id, for messages sent this turn
	aside   []model.Message   // shown to the model on its next call only, never stored
	// turnChars adds up the current turn's tool results, to keep them within maxTurnResults.
	turnChars int
	// reads remembers the parts of files the agent has read since its last compaction.
	reads map[string]readState
	// calibration is the provider's real prompt size over openbot's estimate, from the last call.
	calibration float64
	images      []string // image attachments of the event being rendered

	// The current turn or task run, which the user can stop (see stop.go).
	workMu     sync.Mutex
	cancelWork context.CancelFunc
	stopped    bool
}

func (l *loop) poke() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *loop) run(ctx context.Context) {
	l.poke() // process anything left in the inbox from before a restart
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
			l.drain(ctx)
		case job := <-l.jobs:
			job(ctx)
			l.drain(ctx)
		}
	}
}

// runJobs runs the jobs queued so far.
func (l *loop) runJobs(ctx context.Context) {
	for {
		select {
		case job := <-l.jobs:
			job(ctx)
		default:
			return
		}
	}
}

// drain runs turns until the inbox is empty.
func (l *loop) drain(ctx context.Context) {
	for ctx.Err() == nil {
		l.runJobs(ctx)
		events, err := l.m.store.PendingEvents(ctx, l.agentID)
		if err != nil {
			logger(l.agentID).Error("load pending events", "err", err)
			return
		}
		// Tasks and app events run separately; their reports wait for the next real turn.
		var tasks, rest []store.Event
		for _, e := range events {
			if e.Kind == EventTask {
				tasks = append(tasks, e)
			} else {
				rest = append(rest, e)
			}
		}
		if len(tasks) > 0 {
			l.runTasks(ctx, tasks)
			continue
		}
		if !slices.ContainsFunc(rest, func(e store.Event) bool { return e.Kind != EventTaskReport }) {
			return
		}
		l.turn(ctx, rest)
	}
}

// turn consumes events into the context and runs the model/tool loop until the agent stops
// calling tools. New events that arrive mid-turn are injected between model steps.
func (l *loop) turn(ctx context.Context, events []store.Event) {
	log := logger(l.agentID)
	// The turn works in ctx, which Stop cancels; what it records about that uses base.
	base := ctx
	ctx, finish := l.beginWork(base)
	defer finish()
	agent, err := l.m.store.GetAgent(ctx, l.agentID)
	if err != nil {
		log.Error("load agent", "err", err)
		return
	}

	l.m.setActivity(agent.ID, view.Working("thinking"))
	defer l.m.setActivity(agent.ID, view.Idle())
	// Whoever is waiting on these events (a group's turn coordinator) learns the turn is over.
	l.handled = l.handled[:0]
	defer func() { l.m.eventsHandled(l.handled) }()
	defer func() {
		if l.wasStopped() {
			l.afterStoppedTurn(base, agent)
		}
	}()
	l.fresh = map[string]bool{}
	l.sent = map[string]string{}
	l.aside = nil
	l.turnChars = 0

	added, err := l.consume(ctx, events)
	if err != nil {
		log.Error("consume events", "err", err)
		return
	}
	// Events that rendered to nothing (an empty group turn) don't warrant a model call; a retry
	// renders to nothing on purpose and does.
	if added == 0 && !slices.ContainsFunc(events, func(e store.Event) bool { return e.Kind == EventRetry }) {
		return
	}

	spoke, nudged := false, false
	for range l.m.maxSteps() {
		if ctx.Err() != nil {
			return
		}
		l.maybeCompact(ctx, agent)
		req, err := l.request(ctx, agent)
		if err != nil {
			log.Error("build request", "err", err)
			return
		}
		resp, err := l.m.chat(ctx, agent.ID, "turn", req)
		if err == nil {
			l.calibrate(req, resp.Usage.PromptTokens)
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Warn("model call failed", "err", err)
				l.reportError(ctx, agent, err)
			}
			return
		}
		reply := resp.Message
		reply.Role = "assistant"
		l.aside = nil
		// Plain text before saying anything: point out that nobody saw it, for this one call. The
		// text and the nudge stay out of the context, so a pile of old nudges doesn't teach the
		// model to never end a turn with plain text.
		if len(reply.ToolCalls) == 0 && !spoke && !nudged && strings.TrimSpace(reply.Text()) != "" {
			nudged = true
			l.aside = []model.Message{reply, model.Text("user", nudgeText)}
			continue
		}
		if err := l.appendEntries(base, reply); err != nil {
			log.Error("append reply", "err", err)
			return
		}
		if len(reply.ToolCalls) == 0 {
			return
		}

		asked := false
		delegated := l.prefetchDelegates(ctx, agent, reply.ToolCalls)
		for _, call := range reply.ToolCalls {
			if ctx.Err() != nil {
				break // stopped: the calls left are answered afterwards (afterStoppedTurn)
			}
			call = unwrapAppCall(call)
			call.Function.Arguments = l.fixProse(ctx, agent, call)
			l.m.setActivity(agent.ID, view.Working(l.activity(ctx, agent, call)))
			var result string
			var ok bool
			if l.needsApproval(ctx, agent, call) {
				// Waiting for approval ends the turn like asking a question does.
				result, ok = l.requestApproval(ctx, agent, call)
				asked = asked || ok
				spoke = spoke || ok
			} else {
				if r, done := delegated[call.ID]; done {
					result, ok = r, true
				} else {
					result, ok = l.runTool(ctx, agent, call)
				}
				result = l.limitResult(ctx, agent, call, l.maskSecrets(ctx, agent.ID, result))
			}
			if ctx.Err() != nil {
				result, ok = stoppedResult, false
			}
			asks := call.Function.Name == toolAskUser || call.Function.Name == toolConnectApp || call.Function.Name == toolRequestSecret
			if ok && (asks || call.Function.Name == toolSendMessage || call.Function.Name == toolReact) {
				spoke = true
			}
			if ok && asks {
				asked = true
			}
			if err := l.appendEntries(base, model.Message{
				Role: "tool", Content: &result, ToolCallID: call.ID,
			}); err != nil {
				log.Error("append tool result", "err", err)
				return
			}
		}
		if ctx.Err() != nil {
			return
		}

		// Asking the user a question ends the turn: the answer arrives later as an event. Models
		// don't reliably stop on their own and tend to post filler after asking. So does done.
		if asked || slices.ContainsFunc(reply.ToolCalls, func(c model.ToolCall) bool { return c.Function.Name == toolDone }) {
			return
		}

		// Let the agent notice anything that arrived while it was working.
		pending, err := l.m.store.PendingEvents(ctx, l.agentID)
		if err != nil {
			log.Error("load pending events", "err", err)
			return
		}
		// Tasks that came up meanwhile run on their own once this turn ends.
		pending = slices.DeleteFunc(pending, func(e store.Event) bool { return e.Kind == EventTask })
		if len(pending) > 0 {
			if _, err := l.consume(ctx, pending); err != nil {
				log.Error("consume events", "err", err)
				return
			}
		}
		l.m.setActivity(agent.ID, view.Working("thinking"))
	}
	l.wrapUp(ctx, agent)
	l.reportError(ctx, agent, errTooManySteps)
}

// wrapUp gives an agent that reached the step limit one more call to tell the user where it
// stands. Only messages run; any other tool call is answered as not run.
func (l *loop) wrapUp(ctx context.Context, agent store.Agent) {
	l.aside = []model.Message{model.Text("user", fmt.Sprintf(wrapUpText, l.m.maxSteps()))}
	req, err := l.request(ctx, agent)
	l.aside = nil
	if err != nil {
		return
	}
	resp, err := l.m.chat(ctx, agent.ID, "turn", req)
	if err != nil {
		return
	}
	reply := resp.Message
	reply.Role = "assistant"
	if err := l.appendEntries(ctx, reply); err != nil {
		return
	}
	for _, call := range reply.ToolCalls {
		result := toolError("not run: this turn reached its step limit")
		if call.Function.Name == toolSendMessage {
			call.Function.Arguments = l.fixProse(ctx, agent, call)
			result, _ = l.runTool(ctx, agent, call)
		}
		if err := l.appendEntries(ctx, model.Message{Role: "tool", Content: &result, ToolCallID: call.ID}); err != nil {
			return
		}
	}
}

// consume renders events into the context and marks them consumed. It returns how many context
// entries were added.
func (l *loop) consume(ctx context.Context, events []store.Event) (int, error) {
	ids := make([]string, 0, len(events))
	entries := make([]json.RawMessage, 0, len(events))
	for _, e := range events {
		l.images = nil
		text, err := l.renderEvent(ctx, e)
		if err != nil {
			return 0, fmt.Errorf("render event %s: %w", e.ID, err)
		}
		ids = append(ids, e.ID)
		l.handled = append(l.handled, e.ID)
		if text == "" {
			continue
		}
		text = l.stamp(ctx, text, e.CreatedAt)
		entry := model.Text("user", text)
		entry.ImageRefs = l.images
		b, err := json.Marshal(entry)
		if err != nil {
			return 0, err
		}
		entries = append(entries, b)
	}
	return len(entries), l.m.store.ConsumeEvents(ctx, l.agentID, ids, entries)
}

func (l *loop) appendEntries(ctx context.Context, msgs ...model.Message) error {
	entries := make([]json.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		entries = append(entries, b)
	}
	return l.m.store.AppendContext(ctx, l.agentID, entries...)
}

// request assembles the model request from the system prompt and the agent's context.
func (l *loop) request(ctx context.Context, agent store.Agent) (model.Request, error) {
	msgs, err := l.head(ctx, agent)
	if err != nil {
		return model.Request{}, err
	}
	entries, err := l.m.store.Context(ctx, agent.ID)
	if err != nil {
		return model.Request{}, err
	}
	for _, e := range entries {
		var m model.Message
		if err := json.Unmarshal(e.Entry, &m); err != nil {
			return model.Request{}, fmt.Errorf("context entry %s: %w", e.ID, err)
		}
		msgs = append(msgs, m)
	}
	msgs = append(msgs, l.aside...)
	l.loadImages(ctx, msgs)
	// Each agent is one continuous conversation, so its id is a stable session id.
	return model.Request{Session: "openbot-agent-" + agent.ID, Model: agent.Model, Messages: msgs, Tools: l.tools(ctx, agent)}, nil
}

// head is how every request starts: the frozen system prompt, then the summary of older context.
// Task runs start the same way, so they share the provider's cache of it.
func (l *loop) head(ctx context.Context, agent store.Agent) ([]model.Message, error) {
	system, err := l.systemPrompt(ctx, agent)
	if err != nil {
		return nil, err
	}
	summary, err := l.m.store.ContextSummary(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	msgs := []model.Message{model.Text("system", system)}
	if summary != "" {
		msgs = append(msgs, model.Text("user", "<context_summary>\nA summary of your earlier conversations, "+
			"written by you when they were trimmed from your context:\n\n"+summary+"\n</context_summary>"))
	}
	return msgs, nil
}

// tools are the tools the agent can call: its own, then its apps' (in a stable order).
func (l *loop) tools(ctx context.Context, agent store.Agent) []model.Tool {
	tools := l.m.toolsFor(agent)
	if l.m.Connectors != nil {
		tools = append(tools, connectAppTool())
	}
	apps := asModelTools(ctx, l.m, l.connectorTools(ctx, agent))
	if lazyApps(apps) {
		return append(tools, appToolInfoTool, appToolCallTool)
	}
	return append(tools, apps...)
}

// reportError tells the user, in the agent's DM, that the agent couldn't finish its turn. The
// message carries a structured failure so the client can offer Retry (and Change model).
func (l *loop) reportError(ctx context.Context, agent store.Agent, err error) {
	chatID, dmErr := l.m.store.DMChatID(ctx, agent.ID)
	if dmErr != nil {
		logger(agent.ID).Error("find DM for error report", "err", dmErr)
		return
	}
	reason, text := describeFailure(agent, err, l.m.maxSteps())
	msg, err := l.m.store.InsertFailure(ctx, chatID, text, store.Failure{
		AgentID: agent.ID, Reason: reason, Retryable: true,
	})
	if err != nil {
		logger(agent.ID).Error("post error report", "err", err)
		return
	}
	l.m.bus.Publish(view.MessageCreated(view.Message(msg)))
}

func describeFailure(agent store.Agent, err error, maxSteps int) (reason, text string) {
	switch {
	case errors.Is(err, model.ErrNoKey):
		return "no_key", "No model provider is configured. Add your OpenCode Go API key in Settings, then retry."
	case errors.Is(err, model.ErrInvalidKey):
		return "invalid_key", "OpenCode Go rejected the API key. Replace it in Settings, then retry."
	case errors.Is(err, model.ErrTrainsOnData):
		return "model_blocked", fmt.Sprintf("%s can't use %s: that model's provider trains on request data, "+
			"which your OpenCode workspace's Privacy settings don't allow. Pick another model for %s, "+
			"or allow these models in your OpenCode Privacy settings and retry.", agent.Name, agent.Model, agent.Name)
	case errors.Is(err, errTooManySteps):
		return "too_many_steps", fmt.Sprintf("%s paused after %d steps. Continue to let it keep going.", agent.Name, maxSteps)
	default:
		return "provider_error", fmt.Sprintf("%s couldn't finish responding: %v", agent.Name, err)
	}
}

var openingTag = regexp.MustCompile(`^<([a-z_]+)`)

// stamp adds when an event arrived to its opening tag (received_at), unless it already carries a
// time: the system prompt only has the date, so this is how the agent knows the time.
func (l *loop) stamp(ctx context.Context, text string, at int64) string {
	tag := openingTag.FindStringIndex(text)
	if tag == nil {
		return text
	}
	end := strings.IndexByte(text, '>')
	if end < 0 {
		return text
	}
	if head := text[:end]; strings.Contains(head, "sent_at=") || strings.Contains(head, " at=") || strings.Contains(head, "received_at=") {
		return text
	}
	when := store.Time(at).In(l.m.location(ctx)).Format(time.RFC3339)
	return text[:end] + fmt.Sprintf(" received_at=%q", when) + text[end:]
}

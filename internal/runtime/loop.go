package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

// maxSteps bounds model calls in a single turn.
const maxSteps = 40

var errTooManySteps = errors.New("too many steps in one turn")

const nudgeText = "[system] Your last reply was plain text, which nobody can see. " +
	"If you meant to say something, call send_message. Otherwise end your turn without text."

type loop struct {
	m       *Manager
	agentID string
	wake    chan struct{}
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
		}
	}
}

// drain runs turns until the inbox is empty.
func (l *loop) drain(ctx context.Context) {
	for ctx.Err() == nil {
		events, err := l.m.store.PendingEvents(ctx, l.agentID)
		if err != nil {
			logger(l.agentID).Error("load pending events", "err", err)
			return
		}
		if len(events) == 0 {
			return
		}
		l.turn(ctx, events)
	}
}

// turn consumes events into the context and runs the model/tool loop until the agent stops
// calling tools. New events that arrive mid-turn are injected between model steps.
func (l *loop) turn(ctx context.Context, events []store.Event) {
	log := logger(l.agentID)
	agent, err := l.m.store.GetAgent(ctx, l.agentID)
	if err != nil {
		log.Error("load agent", "err", err)
		return
	}

	l.m.setActivity(agent.ID, view.Working("thinking"))
	defer l.m.setActivity(agent.ID, view.Idle())

	if err := l.consume(ctx, events); err != nil {
		log.Error("consume events", "err", err)
		return
	}

	spoke, nudged := false, false
	for range maxSteps {
		req, err := l.request(ctx, agent)
		if err != nil {
			log.Error("build request", "err", err)
			return
		}
		resp, err := l.m.llm.Chat(ctx, req)
		if err != nil {
			if ctx.Err() == nil {
				log.Warn("model call failed", "err", err)
				l.reportError(ctx, agent, err)
			}
			return
		}
		reply := resp.Message
		reply.Role = "assistant"
		if err := l.appendEntries(ctx, reply); err != nil {
			log.Error("append reply", "err", err)
			return
		}

		if len(reply.ToolCalls) == 0 {
			if !spoke && !nudged && strings.TrimSpace(reply.Text()) != "" {
				nudged = true
				if err := l.appendEntries(ctx, model.Text("user", nudgeText)); err != nil {
					log.Error("append nudge", "err", err)
					return
				}
				continue
			}
			return
		}

		for _, call := range reply.ToolCalls {
			l.m.setActivity(agent.ID, view.Working(toolLabel(call.Function.Name)))
			result, ok := l.runTool(ctx, agent, call)
			if ok && call.Function.Name == toolSendMessage {
				spoke = true
			}
			if err := l.appendEntries(ctx, model.Message{
				Role: "tool", Content: &result, ToolCallID: call.ID,
			}); err != nil {
				log.Error("append tool result", "err", err)
				return
			}
		}

		// Let the agent notice anything that arrived while it was working.
		pending, err := l.m.store.PendingEvents(ctx, l.agentID)
		if err != nil {
			log.Error("load pending events", "err", err)
			return
		}
		if len(pending) > 0 {
			if err := l.consume(ctx, pending); err != nil {
				log.Error("consume events", "err", err)
				return
			}
		}
		l.m.setActivity(agent.ID, view.Working("thinking"))
	}
	l.reportError(ctx, agent, errTooManySteps)
}

func (l *loop) consume(ctx context.Context, events []store.Event) error {
	ids := make([]string, 0, len(events))
	entries := make([]json.RawMessage, 0, len(events))
	for _, e := range events {
		text, err := l.renderEvent(ctx, e)
		if err != nil {
			return fmt.Errorf("render event %s: %w", e.ID, err)
		}
		ids = append(ids, e.ID)
		if text == "" {
			continue
		}
		b, err := json.Marshal(model.Text("user", text))
		if err != nil {
			return err
		}
		entries = append(entries, b)
	}
	return l.m.store.ConsumeEvents(ctx, l.agentID, ids, entries)
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
	system, err := l.systemPrompt(ctx, agent)
	if err != nil {
		return model.Request{}, err
	}
	entries, err := l.m.store.Context(ctx, agent.ID)
	if err != nil {
		return model.Request{}, err
	}
	msgs := make([]model.Message, 0, len(entries)+1)
	msgs = append(msgs, model.Text("system", system))
	for _, e := range entries {
		var m model.Message
		if err := json.Unmarshal(e.Entry, &m); err != nil {
			return model.Request{}, fmt.Errorf("context entry %s: %w", e.ID, err)
		}
		msgs = append(msgs, m)
	}
	// Each agent is one continuous conversation, so its id is a stable session id.
	return model.Request{Session: "barn-agent-" + agent.ID, Model: agent.Model, Messages: msgs, Tools: toolsFor(agent)}, nil
}

// reportError tells the user, in the agent's DM, that the agent couldn't finish its turn. The
// message carries a structured failure so the client can offer Retry (and Change model).
func (l *loop) reportError(ctx context.Context, agent store.Agent, err error) {
	chatID, dmErr := l.m.store.DMChatID(ctx, agent.ID)
	if dmErr != nil {
		logger(agent.ID).Error("find DM for error report", "err", dmErr)
		return
	}
	reason, text := describeFailure(agent, err)
	msg, err := l.m.store.InsertFailure(ctx, chatID, text, store.Failure{
		AgentID: agent.ID, Reason: reason, Retryable: true,
	})
	if err != nil {
		logger(agent.ID).Error("post error report", "err", err)
		return
	}
	l.m.bus.Publish(view.MessageCreated(view.Message(msg)))
}

func describeFailure(agent store.Agent, err error) (reason, text string) {
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
		return "too_many_steps", fmt.Sprintf("%s stopped after %d steps without finishing.", agent.Name, maxSteps)
	default:
		return "provider_error", fmt.Sprintf("%s couldn't finish responding: %v", agent.Name, err)
	}
}

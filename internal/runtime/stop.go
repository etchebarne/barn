package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// The user can stop what an agent is doing right now: its turn or task run ends, along with the
// model call, helpers, and commands in its computer. Work queued after it (new messages, other
// tasks) still runs.

// ErrNotWorking is returned by Stop when the agent isn't doing anything.
var ErrNotWorking = errors.New("agent isn't working")

const stoppedResult = `{"ok":false,"error":"not finished: the user stopped you"}`

// stoppedNote goes into the agent's context after a stop, so it knows why its work ended.
const stoppedNote = "[system] The user pressed Stop, which ended what you were doing (tool calls that " +
	"hadn't finished are marked as stopped). Don't pick it up again unless they ask."

// beginWork derives the context a turn or task run works in, which Stop cancels.
func (l *loop) beginWork(ctx context.Context) (context.Context, func()) {
	work, cancel := context.WithCancel(ctx)
	l.workMu.Lock()
	l.cancelWork, l.stopped = cancel, false
	l.workMu.Unlock()
	return work, func() {
		l.workMu.Lock()
		l.cancelWork = nil
		l.workMu.Unlock()
		cancel()
	}
}

// stopWork cancels the current turn or task run. It reports false when there's none.
func (l *loop) stopWork() bool {
	l.workMu.Lock()
	defer l.workMu.Unlock()
	if l.cancelWork == nil {
		return false
	}
	l.stopped = true
	l.cancelWork()
	return true
}

// wasStopped reports whether the current (or last) work was stopped by the user.
func (l *loop) wasStopped() bool {
	l.workMu.Lock()
	defer l.workMu.Unlock()
	return l.stopped
}

// Stop ends what the agent is doing right now.
func (m *Manager) Stop(agentID string) error {
	m.mu.Lock()
	l := m.loops[agentID]
	m.mu.Unlock()
	if l == nil || !l.stopWork() {
		return ErrNotWorking
	}
	m.setActivity(agentID, view.Working("stopping"))
	return nil
}

// afterStoppedTurn leaves the context valid and the agent informed after a stopped turn: tool
// calls without results get one, and a note says what happened. ctx must not be the cancelled one.
func (l *loop) afterStoppedTurn(ctx context.Context, agent store.Agent) {
	log := logger(agent.ID)
	entries, err := l.m.store.Context(ctx, agent.ID)
	if err != nil {
		log.Error("load context after stop", "err", err)
		return
	}
	msgs := make([]model.Message, 0, len(entries))
	for _, e := range entries {
		var m model.Message
		if json.Unmarshal(e.Entry, &m) == nil {
			msgs = append(msgs, m)
		}
	}
	var fix []model.Message
	if len(msgs) > 0 {
		_, missing := turnState(msgs)
		for _, id := range missing {
			fix = append(fix, model.Message{Role: "tool", Content: ptr(stoppedResult), ToolCallID: id})
		}
	}
	fix = append(fix, model.Text("user", stoppedNote))
	if err := l.appendEntries(ctx, fix...); err != nil {
		log.Error("record stop", "err", err)
	}
	l.postStopped(ctx, agent, "")
}

// postStopped tells the user, in the agent's DM, that it stopped. The notice also keeps a server
// restart from resuming the stopped turn (see resumeIfInterrupted).
func (l *loop) postStopped(ctx context.Context, agent store.Agent, what string) {
	chatID, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return
	}
	text := fmt.Sprintf("You stopped %s.", agent.Name)
	if what != "" {
		text = fmt.Sprintf("You stopped %s's %s.", agent.Name, what)
	}
	msg, err := l.m.store.InsertFailure(ctx, chatID, text, store.Failure{AgentID: agent.ID, Reason: "stopped"})
	if err != nil {
		logger(agent.ID).Error("post stop notice", "err", err)
		return
	}
	l.m.bus.Publish(view.MessageCreated(view.Message(msg)))
}

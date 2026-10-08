package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/model"
)

const interruptedResult = `{"ok":false,"error":"interrupted by a server restart before finishing; check whether it took effect before trying again"}`

// resumeIfInterrupted restarts a turn that a server restart cut short. Inbox events are marked
// consumed when a turn starts, so without this an agent stops mid-turn (e.g. never sends the
// intro it was asked for) until something new happens.
func (m *Manager) resumeIfInterrupted(ctx context.Context, agentID string) error {
	entries, err := m.store.Context(ctx, agentID)
	if err != nil || len(entries) == 0 {
		return err
	}
	msgs := make([]model.Message, len(entries))
	for i, e := range entries {
		if err := json.Unmarshal(e.Entry, &msgs[i]); err != nil {
			return fmt.Errorf("context entry %s: %w", e.ID, err)
		}
	}

	interrupted, missing := turnState(msgs)
	if !interrupted {
		return nil
	}
	// A turn that failed is the user's call to retry (it shows a Retry button).
	if failed, err := m.LastTurnFailed(ctx, agentID); err != nil || failed {
		return err
	}
	// Tool calls without results would make the context invalid for the model.
	for _, id := range missing {
		b, err := json.Marshal(model.Message{Role: "tool", Content: ptr(interruptedResult), ToolCallID: id})
		if err != nil {
			return err
		}
		if err := m.store.AppendContext(ctx, agentID, b); err != nil {
			return err
		}
	}
	logger(agentID).Info("resuming a turn interrupted by a restart")
	// Events already waiting start the next turn, which carries on from the same context.
	if err := m.Retry(ctx, agentID); !errors.Is(err, ErrBusy) {
		return err
	}
	return nil
}

// turnState reports whether the context ends mid-turn, and which tool calls of the last
// assistant message have no result yet.
func turnState(msgs []model.Message) (interrupted bool, missingResults []string) {
	last := msgs[len(msgs)-1]
	switch last.Role {
	case "user":
		return true, nil // input that was never answered
	case "assistant":
		if len(last.ToolCalls) == 0 {
			return false, nil // the agent finished its turn
		}
		for _, tc := range last.ToolCalls {
			missingResults = append(missingResults, tc.ID)
		}
		return true, missingResults
	case "tool":
		// Collect the results that follow the assistant message that made the calls.
		answered := map[string]bool{}
		asked := false
		i := len(msgs) - 1
		for ; i >= 0 && msgs[i].Role == "tool"; i-- {
			answered[msgs[i].ToolCallID] = true
			// Waiting for an approval or a connect card also ends the turn (see loop.go).
			if strings.Contains(msgs[i].Text(), `"status":"waiting_for_`) {
				asked = true
			}
		}
		if i < 0 || msgs[i].Role != "assistant" {
			return true, nil
		}
		for _, tc := range msgs[i].ToolCalls {
			if !answered[tc.ID] {
				missingResults = append(missingResults, tc.ID)
			}
			switch tc.Function.Name {
			case toolAskUser, toolRequestSecret, toolConnectApp, toolDone:
				asked = true // these end the turn
			}
		}
		// Asking the user (or calling done) ends a turn, so a completed reply like that isn't interrupted.
		if asked && len(missingResults) == 0 {
			return false, nil
		}
		return true, missingResults
	default:
		return false, nil
	}
}

func ptr[T any](v T) *T { return &v }

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/sandbox"
)

// blockingSandbox runs commands until they're cancelled.
type blockingSandbox struct {
	fakeSandbox
	started chan struct{}
}

func (b *blockingSandbox) Exec(ctx context.Context, _, _, _ string, _ []byte, _ time.Duration, _ map[string]string) (sandbox.Result, error) {
	close(b.started)
	<-ctx.Done()
	return sandbox.Result{ExitCode: 137}, ctx.Err()
}

func TestStopEndsTheTurn(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message {
			msg := toolCall(toolRunCommand, map[string]string{"command": "sleep 600"})
			msg.ToolCalls = append(msg.ToolCalls, sendCall(f.chatID, "done!").ToolCalls...)
			return msg
		},
	)
	sbx := &blockingSandbox{started: make(chan struct{})}
	f.rt.Sandboxes = sbx
	ctx := context.Background()

	if err := f.rt.Stop(f.agent.ID); !errors.Is(err, ErrNotWorking) {
		t.Fatalf("Stop while idle = %v, want ErrNotWorking", err)
	}
	f.userSays(t, "sleep for a while")
	select {
	case <-sbx.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the command never started")
	}
	if err := f.rt.Stop(f.agent.ID); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	f.waitIdle(t)

	// The calls got results (the running one and the one never run), then a note.
	entries, err := f.store.Context(ctx, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	var tail []model.Message
	for _, e := range entries[len(entries)-3:] {
		var m model.Message
		_ = json.Unmarshal(e.Entry, &m)
		tail = append(tail, m)
	}
	if tail[0].Role != "tool" || tail[0].Text() != stoppedResult || tail[1].Role != "tool" || tail[1].Text() != stoppedResult {
		t.Fatalf("tool results = %+v", tail[:2])
	}
	if tail[2].Role != "user" || tail[2].Text() != stoppedNote {
		t.Fatalf("last entry = %+v", tail[2])
	}
	if f.llm.calls() != 1 {
		t.Fatalf("model calls = %d, want 1", f.llm.calls())
	}

	// Nothing was sent; the DM says the agent was stopped.
	msgs, _, err := f.store.ListMessages(ctx, f.chatID, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	last := msgs[len(msgs)-1]
	if last.Failure == nil || last.Failure.Reason != "stopped" || last.Failure.Retryable {
		t.Fatalf("last message = %+v", last)
	}
	for _, m := range msgs {
		if m.Body == "done!" {
			t.Fatal("a call after the stopped one ran")
		}
	}
	// A restart doesn't pick the stopped turn back up.
	if err := f.rt.resumeIfInterrupted(ctx, f.agent.ID); err != nil {
		t.Fatal(err)
	}
	if pending, _ := f.store.PendingEvents(ctx, f.agent.ID); len(pending) != 0 {
		t.Fatalf("pending after restart check = %+v", pending)
	}
}

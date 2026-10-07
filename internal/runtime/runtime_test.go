package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/barn/internal/bus"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// fakeModel replays scripted responses and records the requests it received.
type fakeModel struct {
	mu        sync.Mutex
	responses []func(req model.Request) model.Message
	requests  []model.Request
	failNext  error
}

func (f *fakeModel) Chat(_ context.Context, req model.Request) (model.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if err := f.failNext; err != nil {
		f.failNext = nil
		return model.Response{}, err
	}
	if len(f.responses) == 0 {
		return model.Response{Message: model.Text("assistant", "")}, nil
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return model.Response{Message: next(req)}, nil
}

func (f *fakeModel) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func sendCall(chatID, text string) model.Message {
	args, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	return model.Message{Role: "assistant", ToolCalls: []model.ToolCall{{
		ID: "call_" + text, Type: "function",
		Function: model.FunctionCall{Name: toolSendMessage, Arguments: string(args)},
	}}}
}

type fixture struct {
	store  *store.Store
	rt     *Manager
	llm    *fakeModel
	agent  store.Agent
	chatID string
}

func setup(t *testing.T, responses ...func(model.Request) model.Message) fixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFirstUser(ctx, "martin", "x"); err != nil {
		t.Fatal(err)
	}
	agent, chatID, err := st.CreateAgentWithDM(ctx, store.Agent{
		Name: "barn", Instructions: "Be helpful.", Model: "test-model", Language: "auto", TrustMode: "ask",
	})
	if err != nil {
		t.Fatal(err)
	}
	llm := &fakeModel{responses: responses}
	rt := New(st, bus.New(), llm)
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		rt.Wait()
		st.Close()
	})
	return fixture{store: st, rt: rt, llm: llm, agent: agent, chatID: chatID}
}

func (f fixture) userSays(t *testing.T, text string) {
	t.Helper()
	ctx := context.Background()
	msg, err := f.store.InsertMessage(ctx, f.chatID, "user", nil, text)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.DeliverUserMessage(ctx, msg); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) waitForMessages(t *testing.T, n int) []store.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		msgs, _, err := f.store.ListMessages(context.Background(), f.chatID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) >= n {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d messages, have %d", n, len(msgs))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (f fixture) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, _ := f.store.PendingEvents(context.Background(), f.agent.ID)
		if len(pending) == 0 && f.rt.Activity(f.agent.ID).State == "idle" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for agent to go idle")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentRepliesViaSendMessage(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message { return sendCall(f.chatID, "hello martin") },
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	f.userSays(t, "hi")
	msgs := f.waitForMessages(t, 2)
	if msgs[1].AuthorKind != "agent" || msgs[1].Body != "hello martin" {
		t.Fatalf("unexpected reply: %+v", msgs[1])
	}

	f.waitIdle(t)
	req := f.llm.requests[0]
	if req.Model != "test-model" {
		t.Fatalf("model = %q", req.Model)
	}
	sys := req.Messages[0].Text()
	if !strings.Contains(sys, "chat_id "+f.chatID+": DM with martin") {
		t.Fatalf("system prompt missing chat roster:\n%s", sys)
	}
	last := req.Messages[len(req.Messages)-1].Text()
	if !strings.Contains(last, `from="martin"`) || !strings.Contains(last, "hi") {
		t.Fatalf("event not rendered as expected: %s", last)
	}
	// The second request must include the tool result for the first call.
	second := f.llm.requests[1].Messages
	if tr := second[len(second)-1]; tr.Role != "tool" || !strings.Contains(tr.Text(), `"ok":true`) {
		t.Fatalf("expected successful tool result, got %+v", tr)
	}
}

func TestPlainTextIsPrivateAndNudged(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message { return model.Text("assistant", "hello (but nobody sees this)") },
		func(req model.Request) model.Message {
			last := req.Messages[len(req.Messages)-1].Text()
			if !strings.Contains(last, "nobody can see") {
				t.Errorf("expected nudge, got %q", last)
			}
			return sendCall(f.chatID, "hello for real")
		},
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	f.userSays(t, "hi")
	msgs := f.waitForMessages(t, 2)
	if msgs[1].Body != "hello for real" {
		t.Fatalf("unexpected reply: %+v", msgs[1])
	}
	f.waitIdle(t)
	if f.llm.calls() != 3 {
		t.Fatalf("expected 3 model calls, got %d", f.llm.calls())
	}
}

func TestMessagesArrivingMidTurnAreInjected(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message {
			// While the agent is "working", the user sends another message.
			f.userSays(t, "also, one more thing")
			return sendCall(f.chatID, "on it")
		},
		func(req model.Request) model.Message {
			last := req.Messages[len(req.Messages)-1].Text()
			if !strings.Contains(last, "one more thing") {
				t.Errorf("expected injected message after tool result, got %q", last)
			}
			return model.Text("assistant", "")
		},
	)
	f.userSays(t, "do the thing")
	f.waitForMessages(t, 3)
	f.waitIdle(t)
	if f.llm.calls() != 2 {
		t.Fatalf("expected the injected message to be handled in the same turn (2 calls), got %d", f.llm.calls())
	}
}

func TestSendMessageRejectsForeignChat(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message { return sendCall("not-a-chat", "hello") },
		func(req model.Request) model.Message {
			tr := req.Messages[len(req.Messages)-1]
			if tr.Role != "tool" || !strings.Contains(tr.Text(), "unknown chat_id") {
				t.Errorf("expected tool error, got %+v", tr)
			}
			return sendCall(f.chatID, "sorry")
		},
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	f.userSays(t, "hi")
	msgs := f.waitForMessages(t, 2)
	if msgs[1].Body != "sorry" {
		t.Fatalf("unexpected reply: %+v", msgs[1])
	}
}

func TestFailedTurnCanBeRetried(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message { return sendCall(f.chatID, "here after all") },
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	f.llm.mu.Lock()
	f.llm.failNext = fmt.Errorf("%w (upstream says no)", model.ErrTrainsOnData)
	f.llm.mu.Unlock()

	f.userSays(t, "hi")
	msgs := f.waitForMessages(t, 2)
	failure := msgs[1].Failure
	if msgs[1].AuthorKind != "system" || failure == nil || failure.Reason != "model_blocked" || !failure.Retryable {
		t.Fatalf("expected a retryable model_blocked failure, got %+v (failure %+v)", msgs[1], failure)
	}
	f.waitIdle(t)
	if failed, _ := f.rt.LastTurnFailed(context.Background(), f.agent.ID); !failed {
		t.Fatal("LastTurnFailed should be true")
	}

	if err := f.rt.Retry(context.Background(), f.agent.ID); err != nil {
		t.Fatal(err)
	}
	msgs = f.waitForMessages(t, 3)
	if msgs[2].Body != "here after all" {
		t.Fatalf("unexpected reply after retry: %+v", msgs[2])
	}
	f.waitIdle(t)

	// The retry re-ran the model on the same context: the user's message is still the last input.
	f.llm.mu.Lock()
	retryReq := f.llm.requests[1]
	f.llm.mu.Unlock()
	last := retryReq.Messages[len(retryReq.Messages)-1]
	if last.Role != "user" || !strings.Contains(last.Text(), "hi") {
		t.Fatalf("retry should resend the same context, last message was %+v", last)
	}
	if failed, _ := f.rt.LastTurnFailed(context.Background(), f.agent.ID); failed {
		t.Fatal("LastTurnFailed should be false after a successful retry")
	}
}

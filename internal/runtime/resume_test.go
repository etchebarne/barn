package runtime

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/etchebarne/openbot/internal/bus"
	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

func calls(names ...string) model.Message {
	m := model.Message{Role: "assistant"}
	for i, n := range names {
		m.ToolCalls = append(m.ToolCalls, model.ToolCall{ID: string(rune('a' + i)), Function: model.FunctionCall{Name: n}})
	}
	return m
}

func result(id string) model.Message {
	m := model.Text("tool", "{}")
	m.ToolCallID = id
	return m
}

// waiting is a tool result for a call that handed the turn to the user.
func waiting(id, status string) model.Message {
	m := model.Text("tool", toolOK(map[string]string{"status": status}))
	m.ToolCallID = id
	return m
}

func TestTurnState(t *testing.T) {
	user := model.Text("user", "hi")
	cases := []struct {
		name        string
		msgs        []model.Message
		interrupted bool
		missing     []string
	}{
		{"unanswered input", []model.Message{user}, true, nil},
		{"finished with text", []model.Message{user, model.Text("assistant", "")}, false, nil},
		{"calls without results", []model.Message{user, calls(toolSendMessage, toolSendMessage)}, true, []string{"a", "b"}},
		{"some results missing", []model.Message{user, calls(toolSendMessage, toolSendMessage), result("a")}, true, []string{"b"}},
		{"all results, mid-turn", []model.Message{user, calls(toolSendMessage), result("a")}, true, nil},
		{"asked the user", []model.Message{user, calls(toolSendMessage, toolAskUser), result("a"), result("b")}, false, nil},
		{"asked, a later call cut off", []model.Message{user, calls(toolAskUser, toolSendMessage), result("a")}, true, []string{"b"}},
		{"waiting for approval", []model.Message{user, calls("github__create_issue"), waiting("a", "waiting_for_approval")}, false, nil},
		{"waiting on a connect card", []model.Message{user, calls(toolConnectApp), waiting("a", "waiting_for_user")}, false, nil},
		{"ended with done", []model.Message{user, calls(toolSendMessage, toolDone), result("a"), result("b")}, false, nil},
		{"asked for a secret", []model.Message{user, calls(toolRequestSecret), result("a")}, false, nil},
	}
	for _, c := range cases {
		interrupted, missing := turnState(c.msgs)
		if interrupted != c.interrupted || !slices.Equal(missing, c.missing) {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, interrupted, missing, c.interrupted, c.missing)
		}
	}
}

// A restart between consuming an event and finishing the turn must not lose the turn.
func TestInterruptedTurnResumesOnStart(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateFirstUser(ctx, "martin", "x")
	agent, chatID, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "Notes", Instructions: "x", Model: "test-model", Language: "auto", TrustMode: "ask"})

	// State left behind by a crash: the intro notice was consumed, nothing else happened.
	ev, _ := st.InsertEvent(ctx, agent.ID, EventSystem, map[string]string{"text": "introduce yourself"})
	entry, _ := json.Marshal(model.Text("user", "<system_notice>\nintroduce yourself\n</system_notice>"))
	if err := st.ConsumeEvents(ctx, agent.ID, []string{ev.ID}, []json.RawMessage{entry}); err != nil {
		t.Fatal(err)
	}

	llm := &fakeModel{responses: []func(model.Request) model.Message{
		func(model.Request) model.Message { return sendCall(chatID, "hi, I'm Notes") },
		func(model.Request) model.Message { return model.Text("assistant", "") },
	}}
	runCtx, cancel := context.WithCancel(ctx)
	rt := New(st, bus.New(), llm)
	if err := rt.Start(runCtx); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); rt.Wait() }()

	f := fixture{store: st, rt: rt, llm: llm, agent: agent, chatID: chatID}
	if msgs := f.waitForMessages(t, 1); msgs[0].Body != "hi, I'm Notes" {
		t.Fatalf("expected the interrupted intro to be sent, got %+v", msgs)
	}
}

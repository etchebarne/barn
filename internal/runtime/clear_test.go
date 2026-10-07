package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/model"
)

func TestClearDM(t *testing.T) {
	ctx := context.Background()
	var f fixture
	f = setup(t, func(model.Request) model.Message {
		return toolCall(toolSendMessage, map[string]string{"chat_id": f.chatID, "text": "noted, the code is 1234"})
	})
	f.userSays(t, "remember the code 1234")
	f.waitIdle(t)

	// Other work in the context: a group turn, a task that DMed the user, a task that didn't.
	add := func(msgs ...model.Message) {
		for _, m := range msgs {
			b, _ := json.Marshal(m)
			if err := f.store.AppendContext(ctx, f.agent.ID, b); err != nil {
				t.Fatal(err)
			}
		}
	}
	tool := func(id, content string) model.Message {
		return model.Message{Role: "tool", Content: &content, ToolCallID: id}
	}
	call := func(id, chat string) model.Message {
		m := toolCall(toolSendMessage, map[string]string{"chat_id": chat, "text": "hi"})
		m.ToolCalls[0].ID = id
		return m
	}
	group := "01M4GROUPCHAT0000000000000"
	add(model.Text("user", `<group_turn chat_id="`+group+`" chat="Launch crew">ship it?</group_turn>`),
		call("g1", group), tool("g1", `{"ok":true}`))
	add(model.Text("user", `<task_fired task_id="t1">Remind them</task_fired>`),
		call("t1", f.chatID), tool("t1", `{"ok":true}`))
	add(model.Text("user", `<task_fired task_id="t2">Tidy notes</task_fired>`), model.Text("assistant", "nothing to do"))
	if err := f.store.Compact(ctx, f.agent.ID, "", "Earlier: the user likes short replies."); err != nil {
		t.Fatal(err)
	}

	if err := f.rt.ClearDM(ctx, f.chatID); err != nil {
		t.Fatal(err)
	}
	if msgs := mustList(t, f); len(msgs) != 0 {
		t.Fatalf("messages left: %+v", msgs)
	}
	var left string
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, _ := f.store.Context(ctx, f.agent.ID)
		var b strings.Builder
		for _, e := range entries {
			b.Write(e.Entry)
			b.WriteByte('\n')
		}
		left = b.String()
		if !strings.Contains(left, "1234") || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Contains(left, "1234") || strings.Contains(left, "Remind them") {
		t.Fatalf("the DM conversation is still in the context:\n%s", left)
	}
	if !strings.Contains(left, "group_turn") || !strings.Contains(left, `"g1"`) || !strings.Contains(left, "Tidy notes") {
		t.Fatalf("unrelated context was dropped:\n%s", left)
	}
	if s, _ := f.store.ContextSummary(ctx, f.agent.ID); s != "" {
		t.Fatalf("an agent in no groups keeps no summary: %q", s)
	}

	// It still works afterwards.
	f.llm.mu.Lock()
	f.llm.responses = append(f.llm.responses, func(model.Request) model.Message {
		return toolCall(toolSendMessage, map[string]string{"chat_id": f.chatID, "text": "hello"})
	})
	f.llm.mu.Unlock()
	f.userSays(t, "hi again")
	f.waitIdle(t)
	if msgs := mustList(t, f); len(msgs) != 2 {
		t.Fatalf("after clearing: %+v", msgs)
	}
	if err := f.rt.ClearDM(ctx, "nope"); err == nil {
		t.Fatal("cleared an unknown chat")
	}
}

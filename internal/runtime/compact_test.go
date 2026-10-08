package runtime

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
)

func TestCompactionCut(t *testing.T) {
	user := model.Text("user", "x")
	reply := model.Text("assistant", "")
	call := calls(toolSendMessage)
	res := result("a")
	msgs := []model.Message{user, call, res, reply, user, call, res, reply, user, reply}
	sizes := []int{10, 10, 10, 10, 10, 10, 10, 10, 10, 10}

	// Keeping ~25 tokens lands mid-turn (index 7); the cut moves forward to the next turn start.
	if got := compactionCut(msgs, sizes, 25); got != 8 {
		t.Fatalf("cut = %d, want 8", got)
	}
	// Keeping ~45 tokens lands on index 5 (a tool call); next turn start is 8.
	if got := compactionCut(msgs, sizes, 45); got != 8 {
		t.Fatalf("cut = %d, want 8", got)
	}
	// Keeping everything means nothing to compact.
	if got := compactionCut(msgs, sizes, 1000); got != 0 {
		t.Fatalf("cut = %d, want 0", got)
	}
	// The cut never separates a tool call from its result.
	for keep := 0; keep <= 100; keep += 5 {
		cut := compactionCut(msgs, sizes, keep)
		if cut > 0 && msgs[cut].Role != "user" {
			t.Fatalf("keep %d: cut %d isn't a turn boundary (%s)", keep, cut, msgs[cut].Role)
		}
	}
}

// Long conversations get summarized: old entries are replaced by a summary that the agent
// sees on later turns, and recent turns stay verbatim.
func TestContextIsCompacted(t *testing.T) {
	var f fixture
	f = setup(t)
	// The system prompt and tool definitions alone are ~1.5k tokens.
	f.rt.CompactAtTokens = 3000

	var mu sync.Mutex
	var compactions, turns int
	var lastTurn model.Request
	f.llm.handler = func(req model.Request) model.Message {
		mu.Lock()
		defer mu.Unlock()
		if req.Messages[0].Text() == compactorPrompt {
			compactions++
			input := req.Messages[1].Text()
			if compactions == 1 && !strings.Contains(input, "message number 0") {
				t.Errorf("the first compaction should fold in the oldest messages")
			}
			if compactions > 1 && !strings.Contains(input, "## Previous summary") {
				t.Errorf("later compactions should build on the previous summary")
			}
			return model.Text("assistant", "- Martin sent a numbered series of messages.")
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "user" {
			turns++
			lastTurn = req
			return sendCall(f.chatID, "got it")
		}
		return model.Text("assistant", "")
	}

	for i := range 12 {
		f.userSays(t, "message number "+strings.Repeat("x", 300)+" "+string(rune('a'+i))+" message number "+itoa(i))
		f.waitIdle(t)
	}

	mu.Lock()
	defer mu.Unlock()
	if compactions == 0 {
		t.Fatal("expected at least one compaction")
	}
	summary, _ := f.store.ContextSummary(context.Background(), f.agent.ID)
	if summary == "" {
		t.Fatal("expected a stored summary")
	}
	if got := lastTurn.Messages[1].Text(); !strings.Contains(got, "<context_summary>") || !strings.Contains(got, "numbered series") {
		t.Fatalf("expected the summary right after the system prompt, got %q", got)
	}
	entries, _ := f.store.Context(context.Background(), f.agent.ID)
	if len(entries) >= 12*4 {
		t.Fatalf("expected old entries to be removed, still have %d", len(entries))
	}
	if !strings.Contains(lastTurn.Messages[len(lastTurn.Messages)-1].Text(), "message number 11") {
		t.Fatal("the latest message must stay verbatim")
	}
	// Chat history itself is untouched.
	msgs, _, _ := f.store.ListMessages(context.Background(), f.chatID, "", 100)
	if len(msgs) != 24 {
		t.Fatalf("expected all 24 chat messages to remain, got %d", len(msgs))
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestMemories(t *testing.T) {
	var f fixture
	var memoryID string
	f = setup(t,
		func(model.Request) model.Message {
			return toolCall(toolRemember, map[string]string{"text": "Martin prefers lowercase replies.", "chat_id": f.chatID})
		},
		func(req model.Request) model.Message {
			res := req.Messages[len(req.Messages)-1].Text()
			memoryID = strings.Split(strings.Split(res, `"memory_id":"`)[1], `"`)[0]
			// The prompt is frozen: a memory the agent just saved waits for the next rebuild.
			if strings.Contains(req.Messages[0].Text(), "Martin prefers lowercase replies.") {
				t.Errorf("saving a memory shouldn't rebuild the system prompt")
			}
			return model.Text("assistant", "")
		},
		func(req model.Request) model.Message {
			if !strings.Contains(req.Messages[0].Text(), "Martin prefers lowercase replies.") {
				t.Errorf("memories should be in the system prompt after a rebuild")
			}
			return toolCall(toolForget, map[string]string{"memory_id": memoryID})
		},
		func(model.Request) model.Message { return model.Text("assistant", "") },
		func(req model.Request) model.Message {
			if strings.Contains(req.Messages[0].Text(), "lowercase") {
				t.Errorf("forgotten memory still in the system prompt")
			}
			return model.Text("assistant", "")
		},
	)
	f.userSays(t, "please always reply in lowercase")
	f.waitIdle(t)
	mems, _ := f.store.Memories(context.Background(), f.agent.ID)
	if len(mems) != 1 || mems[0].SourceChatID == nil || *mems[0].SourceChatID != f.chatID {
		t.Fatalf("expected one memory from this chat, got %+v", mems)
	}
	// A rebuild (compaction, or the user editing memories) brings it in.
	f.store.InvalidatePrompt(context.Background(), f.agent.ID)
	f.userSays(t, "actually, forget that")
	f.waitIdle(t)
	f.store.InvalidatePrompt(context.Background(), f.agent.ID)
	f.userSays(t, "hi")
	f.waitIdle(t)
	if mems, _ := f.store.Memories(context.Background(), f.agent.ID); len(mems) != 0 {
		t.Fatalf("expected the memory to be forgotten, have %+v", mems)
	}
}

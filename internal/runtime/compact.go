package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

// DefaultCompactAtTokens is when an agent's context gets compacted. Models served by OpenCode Go
// have context windows of 128k tokens or more; compacting well before that keeps requests fast
// and cheap and leaves room for long tool results.
const DefaultCompactAtTokens = 80_000

// keepFraction of the compaction threshold stays verbatim (the most recent turns).
const keepFraction = 0.3

const compactorPrompt = `You maintain the long-term working memory of an AI agent. You'll get the
agent's previous summary (if any) and the oldest part of its conversation log, which is about to be
removed from its context. Write the new summary that replaces both.

Keep, per chat (name the chat and its chat_id):
- what was discussed and decided, and why
- open questions, promises the agent made, and work in progress
- facts about the people and their preferences that matter later
- prompts the agent asked that are still unanswered

Drop greetings, filler, and anything superseded. Write compact Markdown bullets, under 1,200 words,
in the agent's own voice ("I told Martin…"). Output only the summary.`

// estimateTokens approximates a request's prompt size. JSON overhead makes this an overestimate,
// which errs on the side of compacting early.
func estimateTokens(req model.Request) int {
	b, _ := json.Marshal(req)
	return len(b) / 4
}

// maybeCompact summarizes the oldest part of the agent's context when its next request would be
// too large. Failures are logged and the turn continues with the full context.
func (l *loop) maybeCompact(ctx context.Context, agent store.Agent) {
	limit := l.m.compactAt()
	req, err := l.request(ctx, agent)
	if err != nil || estimateTokens(req) < limit {
		return
	}
	if err := l.compact(ctx, agent, limit); err != nil && ctx.Err() == nil {
		logger(agent.ID).Warn("compaction failed; continuing with the full context", "err", err)
	}
}

func (l *loop) compact(ctx context.Context, agent store.Agent, limit int) error {
	entries, err := l.m.store.Context(ctx, agent.ID)
	if err != nil {
		return err
	}
	msgs := make([]model.Message, len(entries))
	sizes := make([]int, len(entries))
	for i, e := range entries {
		if err := json.Unmarshal(e.Entry, &msgs[i]); err != nil {
			return fmt.Errorf("context entry %s: %w", e.ID, err)
		}
		sizes[i] = len(e.Entry) / 4
	}
	cut := compactionCut(msgs, sizes, int(float64(limit)*keepFraction))
	if cut <= 0 {
		return nil // nothing old enough to compact
	}

	previous, err := l.m.store.ContextSummary(ctx, agent.ID)
	if err != nil {
		return err
	}
	var transcript strings.Builder
	if previous != "" {
		transcript.WriteString("## Previous summary\n\n" + previous + "\n\n")
	}
	transcript.WriteString("## Conversation log to fold in\n\n")
	for _, m := range msgs[:cut] {
		renderForSummary(&transcript, m)
	}

	l.m.setActivity(agent.ID, view.Working("tidying up my notes"))
	resp, err := l.m.llm.Chat(ctx, model.Request{
		Session:   "barn-agent-" + agent.ID,
		Model:     agent.Model,
		Messages:  []model.Message{model.Text("system", compactorPrompt), model.Text("user", transcript.String())},
		MaxTokens: 4000,
	})
	if err != nil {
		return err
	}
	summary := strings.TrimSpace(resp.Message.Text())
	if summary == "" {
		return fmt.Errorf("compactor returned an empty summary")
	}
	logger(agent.ID).Info("compacted context", "entries", cut, "kept", len(msgs)-cut)
	return l.m.store.Compact(ctx, agent.ID, entries[cut].ID, summary)
}

// compactionCut returns the index of the first entry to keep: the most recent entries totaling
// about keep tokens, starting at a user entry so tool calls are never separated from their
// results. 0 means nothing should be compacted.
func compactionCut(msgs []model.Message, sizes []int, keep int) int {
	kept := 0
	cut := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		kept += sizes[i]
		if kept > keep {
			break
		}
		cut = i
	}
	// Move forward to the next turn boundary (an incoming event); if none, back to the last one.
	for i := cut; i < len(msgs); i++ {
		if msgs[i].Role == "user" {
			return i
		}
	}
	for i := min(cut, len(msgs)) - 1; i > 0; i-- {
		if msgs[i].Role == "user" {
			return i
		}
	}
	return 0
}

func renderForSummary(b *strings.Builder, m model.Message) {
	switch m.Role {
	case "user":
		b.WriteString(m.Text() + "\n\n")
	case "assistant":
		if t := strings.TrimSpace(m.Text()); t != "" {
			b.WriteString("[my private note] " + t + "\n")
		}
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(b, "[I called %s] %s\n", tc.Function.Name, truncate(tc.Function.Arguments, 1500))
		}
		b.WriteString("\n")
	case "tool":
		fmt.Fprintf(b, "[result] %s\n\n", truncate(m.Text(), 500))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// DefaultCompactAtTokens caps when an agent's context gets compacted. OpenCode Go's models have
// windows of 128k to 1M tokens, but every step resends the whole context, so compacting well
// before that keeps steps fast and cheap.
const DefaultCompactAtTokens = 64_000

// The most recent part of the context stays verbatim: a share of the model's window, between
// tailMinTokens and tailMaxTokens, and at least tailMinEntries entries.
const (
	tailWindowShare = 0.025
	tailMinTokens   = 10_000
	tailMaxTokens   = 25_000
	tailMinEntries  = 8
)

// prunable is how big an old tool result must be to be trimmed before summarizing.
const prunable = 1_000

const compactorPrompt = `You maintain the long-term working memory of an AI agent. You'll get the
agent's previous summary (if any) and the oldest part of its conversation log, which is about to be
removed from its context. Write the new summary that replaces both: keep everything from the
previous summary that still matters, and fold the new log in.

Use these sections (skip empty ones), as compact Markdown bullets in the agent's own voice
("I told Martin…"):
- **Goals**: what the user wants from me, ongoing.
- **Done**: what I finished, as dated past-tense facts ("Oct 8: posted the release notes to
  #general"), so I don't redo it.
- **In progress**: work started but not finished, and what's next.
- **Promises and open questions**: what I said I'd do, questions I asked that are still
  unanswered, per chat (name the chat and its chat_id).
- **Decisions**: what was decided, and why.
- **People and preferences**: facts about the people that matter later.
- **Errors and fixes**: what went wrong and what worked.
- **Files and places**: paths, URLs, ids I'll need again.

Drop greetings, filler, and anything superseded. Stay under 1,500 words. Output only the summary.`

// estimateTokens approximates a request's prompt size. JSON overhead makes this an overestimate,
// which errs on the side of compacting early.
func estimateTokens(req model.Request) int {
	b, _ := json.Marshal(req)
	return len(b) / 4
}

// maybeCompact summarizes the oldest part of the agent's context when its next request would be
// too large. Failures are logged and the turn continues with the full context.
func (l *loop) maybeCompact(ctx context.Context, agent store.Agent) {
	limit := l.m.compactAt(agent.Model)
	req, err := l.request(ctx, agent)
	if err != nil || l.tokens(req) < limit {
		return
	}
	if err := l.compact(ctx, agent, limit); err != nil && ctx.Err() == nil {
		logger(agent.ID).Warn("compaction failed; continuing with the full context", "err", err)
	}
}

// tokens estimates a request's prompt size, corrected by how the last estimate compared to
// what the provider actually reported.
func (l *loop) tokens(req model.Request) int {
	est := estimateTokens(req)
	if l.calibration > 0 {
		return int(float64(est) * l.calibration)
	}
	return est
}

// calibrate records how a request's estimate compared to the provider's real count.
func (l *loop) calibrate(req model.Request, real int) {
	if est := estimateTokens(req); real > 0 && est > 0 {
		l.calibration = float64(real) / float64(est)
	}
}

// tailTokens is how much of the most recent context stays verbatim.
func (l *loop) tailTokens(agent store.Agent, limit int) int {
	tail := int(float64(l.m.contextWindow(agent.Model)) * tailWindowShare)
	return min(max(tail, tailMinTokens), tailMaxTokens, limit/2)
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
	cut := compactionCut(msgs, sizes, l.tailTokens(agent, limit))
	if cut <= 0 {
		return nil // nothing old enough to compact
	}

	// First the cheap way: trim old big tool results. If that frees enough, no summary needed.
	if pruned := pruneToolResults(msgs[:cut]); len(pruned) > 0 {
		replace := map[string]json.RawMessage{}
		total := 0
		for i := range msgs {
			if i < cut {
				if _, ok := pruned[i]; ok {
					b, err := json.Marshal(msgs[i])
					if err != nil {
						return err
					}
					replace[entries[i].ID] = b
					sizes[i] = len(b) / 4
				}
			}
			total += sizes[i]
		}
		if err := l.m.store.ReplaceContextEntries(ctx, agent.ID, replace); err != nil {
			return err
		}
		l.reads = nil
		logger(agent.ID).Info("trimmed old tool results", "count", len(replace), "context_tokens", total)
		if float64(total)*max(l.calibration, 0.5) < float64(limit)*0.7 {
			return nil
		}
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
	text := l.maskSecrets(ctx, agent.ID, transcript.String())

	l.m.setActivity(agent.ID, view.Working("tidying up my notes"))
	resp, err := l.m.chat(ctx, agent.ID, "compaction", model.Request{
		Session:   "openbot-agent-" + agent.ID,
		Model:     agent.Model,
		Messages:  []model.Message{model.Text("system", compactorPrompt), model.Text("user", text)},
		MaxTokens: 6000,
	})
	if err != nil {
		return err
	}
	summary := l.maskSecrets(ctx, agent.ID, strings.TrimSpace(resp.Message.Text()))
	if summary == "" {
		return fmt.Errorf("compactor returned an empty summary")
	}
	logger(agent.ID).Info("compacted context", "entries", cut, "kept", len(msgs)-cut)
	if err := l.m.store.Compact(ctx, agent.ID, entries[cut].ID, summary); err != nil {
		return err
	}
	// What the agent read may have been summarized away: let it read files again.
	l.reads = nil
	// The cache restarts here anyway: a good moment to bring memories in the prompt up to date.
	return l.m.store.InvalidatePrompt(ctx, agent.ID)
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
	// Keep at least a few entries, however big.
	cut = min(cut, max(len(msgs)-tailMinEntries, 0))
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

// pruneToolResults replaces big tool results with a one-line stub (the call they answer stays,
// so the context remains valid) and returns which ones it changed.
func pruneToolResults(msgs []model.Message) map[int]bool {
	calls := map[string]model.ToolCall{}
	changed := map[int]bool{}
	for i, m := range msgs {
		for _, tc := range m.ToolCalls {
			calls[tc.ID] = tc
		}
		if m.Role != "tool" || len(m.Text()) <= prunable {
			continue
		}
		stub := toolResultStub(calls[m.ToolCallID], m.Text())
		msgs[i].Content = &stub
		changed[i] = true
	}
	return changed
}

// toolResultStub sums up a trimmed tool result.
func toolResultStub(call model.ToolCall, result string) string {
	name := call.Function.Name
	if name == "" {
		name = "tool"
	}
	var parsed struct {
		Result struct {
			ExitCode   *int   `json:"exit_code"`
			FullOutput string `json:"full_output"`
			FullResult string `json:"full_result"`
			Path       string `json:"path"`
		} `json:"result"`
	}
	_ = json.Unmarshal([]byte(result), &parsed)
	var b strings.Builder
	fmt.Fprintf(&b, "[trimmed to save space: %s result, %d characters", name, len(result))
	if args := strings.TrimSpace(call.Function.Arguments); args != "" && args != "{}" {
		fmt.Fprintf(&b, "; called with %s", truncate(args, 200))
	}
	if r := parsed.Result; r.ExitCode != nil {
		fmt.Fprintf(&b, "; exit %d", *r.ExitCode)
	}
	if p := parsed.Result.FullOutput + parsed.Result.FullResult; p != "" {
		fmt.Fprintf(&b, "; saved in %s", p)
	}
	b.WriteString(". Run it again if you need it.]")
	return b.String()
}

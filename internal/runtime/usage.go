package runtime

import (
	"context"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

// DefaultContextWindow is assumed for models whose window isn't known.
const DefaultContextWindow = 128_000

// chat calls the model and records what the call cost.
func (m *Manager) chat(ctx context.Context, agentID, purpose string, req model.Request) (model.Response, error) {
	resp, err := m.llm.Chat(ctx, req)
	if err != nil {
		return resp, err
	}
	u := resp.Usage
	if err := m.store.RecordUsage(ctx, store.UsageRecord{
		AgentID: agentID, Model: req.Model, Purpose: purpose,
		PromptTokens: u.PromptTokens, CachedTokens: u.CachedTokens, CacheWriteTokens: u.CacheWriteTokens,
		CompletionTokens: u.CompletionTokens, ReasoningTokens: u.ReasoningTokens,
	}); err != nil {
		logger(agentID).Warn("record usage", "err", err)
	}
	return resp, nil
}

// contextWindow is a model's context window in tokens.
func (m *Manager) contextWindow(modelID string) int {
	if m.ContextWindow != nil {
		if n := m.ContextWindow(modelID); n > 0 {
			return n
		}
	}
	return DefaultContextWindow
}

// UsageTotals adds up model calls.
type UsageTotals struct {
	Calls, PromptTokens, CachedTokens, CacheWriteTokens, CompletionTokens, ReasoningTokens int
}

func (t *UsageTotals) add(u store.UsageRecord) {
	t.Calls++
	t.PromptTokens += u.PromptTokens
	t.CachedTokens += u.CachedTokens
	t.CacheWriteTokens += u.CacheWriteTokens
	t.CompletionTokens += u.CompletionTokens
	t.ReasoningTokens += u.ReasoningTokens
}

// DayUsage is one day's totals, in the user's time zone ("2026-10-08").
type DayUsage struct {
	Date string
	UsageTotals
}

// UsageReport is what an agent (or all agents) used over recent days.
type UsageReport struct {
	Today     UsageTotals
	Total     UsageTotals
	Days      []DayUsage             // oldest first, one per day including empty ones
	ByPurpose map[string]UsageTotals // turn, task, compaction, subagent
	ByAgent   map[string]UsageTotals // for all agents
	// The agent's context: how big its last request was, its model's window, and when it gets
	// compacted (single agent only).
	ContextTokens, ContextWindow, CompactAt int
}

// Usage reports model usage for the last days days; agentID "" means all agents.
func (m *Manager) Usage(ctx context.Context, agentID string, days int) (UsageReport, error) {
	loc := m.location(ctx)
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	records, err := m.store.UsageSince(ctx, agentID, start.UnixMilli())
	if err != nil {
		return UsageReport{}, err
	}
	r := UsageReport{ByPurpose: map[string]UsageTotals{}, ByAgent: map[string]UsageTotals{}}
	index := map[string]int{}
	for d := 0; d < days; d++ {
		date := start.AddDate(0, 0, d).Format(time.DateOnly)
		index[date] = d
		r.Days = append(r.Days, DayUsage{Date: date})
	}
	today := now.Format(time.DateOnly)
	for _, u := range records {
		date := time.UnixMilli(u.CreatedAt).In(loc).Format(time.DateOnly)
		if i, ok := index[date]; ok {
			r.Days[i].add(u)
		}
		if date == today {
			r.Today.add(u)
		}
		r.Total.add(u)
		p := r.ByPurpose[u.Purpose]
		p.add(u)
		r.ByPurpose[u.Purpose] = p
		if u.AgentID != "" {
			a := r.ByAgent[u.AgentID]
			a.add(u)
			r.ByAgent[u.AgentID] = a
		}
	}
	if agentID != "" {
		agent, err := m.store.GetAgent(ctx, agentID)
		if err != nil {
			return r, err
		}
		if r.ContextTokens, err = m.store.LastPromptTokens(ctx, agentID); err != nil {
			return r, err
		}
		r.ContextWindow, r.CompactAt = m.contextWindow(agent.Model), m.compactAt(agent.Model)
	}
	return r, nil
}

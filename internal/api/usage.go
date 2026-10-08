package api

import (
	"errors"
	"net/http"
	"slices"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/store"
)

func usageDays(days *int) int {
	if days == nil || *days < 1 {
		return 7
	}
	return min(*days, 90)
}

func (s *Server) GetAgentUsage(w http.ResponseWriter, r *http.Request, agentID string, params gen.GetAgentUsageParams) {
	report, err := s.runtime.Usage(r.Context(), agentID, usageDays(params.Days))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	out := usageView(report)
	out.Context = &struct {
		CompactAt int `json:"compactAt"`
		Tokens    int `json:"tokens"`
		Window    int `json:"window"`
	}{CompactAt: report.CompactAt, Tokens: report.ContextTokens, Window: report.ContextWindow}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) GetUsage(w http.ResponseWriter, r *http.Request, params gen.GetUsageParams) {
	report, err := s.runtime.Usage(r.Context(), "", usageDays(params.Days))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usageView(report))
}

func totalsView(t runtime.UsageTotals) gen.UsageTotals {
	return gen.UsageTotals{Calls: t.Calls, PromptTokens: t.PromptTokens, CachedTokens: t.CachedTokens,
		CacheWriteTokens: t.CacheWriteTokens, CompletionTokens: t.CompletionTokens, ReasoningTokens: t.ReasoningTokens}
}

func usageView(r runtime.UsageReport) gen.UsageReport {
	out := gen.UsageReport{
		Today: totalsView(r.Today), Total: totalsView(r.Total),
		Days: []gen.DayUsage{}, ByPurpose: []gen.PurposeUsage{}, ByAgent: []gen.AgentUsage{},
	}
	for _, d := range r.Days {
		t := totalsView(d.UsageTotals)
		date, _ := time.Parse(time.DateOnly, d.Date)
		out.Days = append(out.Days, gen.DayUsage{Date: openapi_types.Date{Time: date}, Calls: t.Calls,
			PromptTokens: t.PromptTokens, CachedTokens: t.CachedTokens, CacheWriteTokens: t.CacheWriteTokens,
			CompletionTokens: t.CompletionTokens, ReasoningTokens: t.ReasoningTokens})
	}
	for purpose, tot := range r.ByPurpose {
		t := totalsView(tot)
		out.ByPurpose = append(out.ByPurpose, gen.PurposeUsage{Purpose: purpose, Calls: t.Calls,
			PromptTokens: t.PromptTokens, CachedTokens: t.CachedTokens, CacheWriteTokens: t.CacheWriteTokens,
			CompletionTokens: t.CompletionTokens, ReasoningTokens: t.ReasoningTokens})
	}
	slices.SortFunc(out.ByPurpose, func(a, b gen.PurposeUsage) int { return b.PromptTokens - a.PromptTokens })
	for id, tot := range r.ByAgent {
		t := totalsView(tot)
		out.ByAgent = append(out.ByAgent, gen.AgentUsage{AgentId: id, Calls: t.Calls,
			PromptTokens: t.PromptTokens, CachedTokens: t.CachedTokens, CacheWriteTokens: t.CacheWriteTokens,
			CompletionTokens: t.CompletionTokens, ReasoningTokens: t.ReasoningTokens})
	}
	slices.SortFunc(out.ByAgent, func(a, b gen.AgentUsage) int { return b.PromptTokens - a.PromptTokens })
	return out
}

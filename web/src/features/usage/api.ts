import { keepPreviousData, useQuery } from "@tanstack/react-query"

import { api, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export type UsageReport = Schemas["UsageReport"]
export type UsageTotals = Schemas["UsageTotals"]
export type DayUsage = Schemas["DayUsage"]
export type PurposeUsage = Schemas["PurposeUsage"]
export type AgentUsage = Schemas["AgentUsage"]

// Usage grows with every model call and isn't pushed over the WebSocket: refetch whenever a
// screen showing it opens or the window regains focus.
const fresh = {
  staleTime: 0,
  refetchOnMount: "always",
  refetchOnWindowFocus: true,
} as const

/** One agent's usage over the last `days` days (including today), plus its context right now. */
export function useAgentUsage(agentId: string, days: number) {
  return useQuery({
    queryKey: queryKeys.agentUsage(agentId, days),
    queryFn: () =>
      unwrap(
        api.GET("/agents/{agentId}/usage", { params: { path: { agentId }, query: { days } } }),
      ),
    ...fresh,
  })
}

/** Usage across all agents over the last `days` days (including today). */
export function useUsage(days: number) {
  return useQuery({
    queryKey: queryKeys.usage(days),
    queryFn: () => unwrap(api.GET("/usage", { params: { query: { days } } })),
    // Switching the period keeps the current numbers until the new ones arrive.
    placeholderData: keepPreviousData,
    ...fresh,
  })
}

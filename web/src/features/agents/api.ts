import { queryOptions, useQuery } from "@tanstack/react-query"

import { api, unwrap, type Agent } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export const agentsQueryOptions = queryOptions({
  queryKey: queryKeys.agents,
  queryFn: () => unwrap(api.GET("/agents")),
})

export function useAgents() {
  return useQuery(agentsQueryOptions)
}

/** Look up agents by id from the (realtime-updated) agents list. */
export function useAgentsById(): Map<string, Agent> {
  const { data } = useAgents()
  const map = new Map<string, Agent>()
  for (const agent of data ?? []) map.set(agent.id, agent)
  return map
}

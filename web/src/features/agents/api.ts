import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Agent, type Schemas } from "@/lib/api-client"
import { updateAgentInCache } from "@/lib/chat-cache"
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

export type AgentChanges = Schemas["UpdateAgentRequest"]

/** PATCH an agent. 400: unknown model; 502: the model list couldn't be loaded. */
export function useUpdateAgent(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (changes: AgentChanges) =>
      unwrap(api.PATCH("/agents/{agentId}", { params: { path: { agentId } }, body: changes })),
    onSuccess: (agent) => updateAgentInCache(queryClient, agent),
  })
}

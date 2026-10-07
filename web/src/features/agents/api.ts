import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, ApiError, unwrap, type Agent, type Schemas } from "@/lib/api-client"
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

/**
 * Re-runs the agent's failed turn. Results arrive over the WebSocket. A 409 means the agent is
 * already working, which is what the user wanted anyway, so it isn't an error.
 */
export function useRetryAgent(agentId: string) {
  return useMutation({
    mutationFn: async () => {
      try {
        await unwrap(api.POST("/agents/{agentId}/retry", { params: { path: { agentId } } }))
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) return
        throw error
      }
    },
  })
}

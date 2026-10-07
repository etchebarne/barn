import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { agentsQueryOptions } from "@/lib/agents"
import { api, ApiError, unwrap, type Agent, type Schemas } from "@/lib/api-client"
import { removeArchivedAgent, removeDeletedAgent, updateAgentInCache } from "@/lib/chat-cache"
import { useLeaveRemovedChats } from "@/lib/leave-removed-chats"
import { queryKeys } from "@/lib/query-keys"

import { useAgentDetailsStore } from "./details-store"
import type { Task } from "./schedule"

export { agentsQueryOptions }

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

export type Memory = Schemas["Memory"]

/** An agent's saved memories. Refetched whenever the details sheet opens. */
export function useMemories(agentId: string) {
  return useQuery({
    queryKey: queryKeys.memories(agentId),
    queryFn: () => unwrap(api.GET("/agents/{agentId}/memories", { params: { path: { agentId } } })),
    staleTime: 0,
  })
}

/** Deletes a memory, removing it from the list right away and restoring it if that fails. */
export function useDeleteMemory(agentId: string) {
  const queryClient = useQueryClient()
  const key = queryKeys.memories(agentId)
  return useMutation({
    mutationFn: (memoryId: string) =>
      unwrap(
        api.DELETE("/agents/{agentId}/memories/{memoryId}", {
          params: { path: { agentId, memoryId } },
        }),
      ),
    onMutate: async (memoryId) => {
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData<Memory[]>(key)
      queryClient.setQueryData<Memory[]>(key, (list) => list?.filter((m) => m.id !== memoryId))
      return { previous }
    },
    onError: (_error, _id, context) => queryClient.setQueryData(key, context?.previous),
    onSettled: () => queryClient.invalidateQueries({ queryKey: key }),
  })
}

/** Archives an agent. 409: it can't be archived (e.g. the last admin agent). */
export function useArchiveAgent(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    // Doesn't wait for the WebSocket: drops the agent and its DM right away, and resolves to the
    // removed chat ids.
    mutationFn: async () => {
      await unwrap(api.POST("/agents/{agentId}/archive", { params: { path: { agentId } } }))
      return removeArchivedAgent(queryClient, agentId)
    },
  })
}

/**
 * Deletes an agent for good. The cache, toast, closing the sheet and leaving its DM happen
 * here (not in the caller) so they still run if the sheet unmounts because the WebSocket's
 * `agent.deleted` removed the agent first. 409: it's the last admin agent.
 */
export function useDeleteAgent(agent: Pick<Agent, "id" | "name">) {
  const queryClient = useQueryClient()
  const leaveRemovedChats = useLeaveRemovedChats()
  return useMutation({
    mutationFn: async () => {
      await unwrap(api.DELETE("/agents/{agentId}", { params: { path: { agentId: agent.id } } }))
      return removeDeletedAgent(queryClient, agent.id)
    },
    onSuccess: (removedChatIds) => {
      toast.success(`Deleted ${agent.name}`)
      useAgentDetailsStore.getState().close()
      leaveRemovedChats(removedChatIds)
    },
  })
}

export function useTasks(agentId: string) {
  return useQuery({
    queryKey: queryKeys.tasks(agentId),
    queryFn: () => unwrap(api.GET("/agents/{agentId}/tasks", { params: { path: { agentId } } })),
    // Agents change their tasks from chat; always refresh when the sheet opens.
    staleTime: 0,
    refetchOnMount: "always",
  })
}

/** Pause or resume a task (optimistic). */
export function useSetTaskEnabled(agentId: string) {
  const queryClient = useQueryClient()
  const key = queryKeys.tasks(agentId)
  return useMutation({
    mutationFn: ({ taskId, enabled }: { taskId: string; enabled: boolean }) =>
      unwrap(api.PATCH("/tasks/{taskId}", { params: { path: { taskId } }, body: { enabled } })),
    onMutate: ({ taskId, enabled }) => {
      const previous = queryClient.getQueryData<Task[]>(key)
      queryClient.setQueryData<Task[]>(key, (tasks) =>
        tasks?.map((t) => (t.id === taskId ? { ...t, enabled } : t)),
      )
      return { previous }
    },
    onSuccess: (task) =>
      queryClient.setQueryData<Task[]>(key, (tasks) =>
        tasks?.map((t) => (t.id === task.id ? task : t)),
      ),
    onError: (_error, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(key, context.previous)
    },
  })
}

export function useDeleteTask(agentId: string) {
  const queryClient = useQueryClient()
  const key = queryKeys.tasks(agentId)
  return useMutation({
    mutationFn: (taskId: string) =>
      unwrap(api.DELETE("/tasks/{taskId}", { params: { path: { taskId } } })),
    onSuccess: (_data, taskId) =>
      queryClient.setQueryData<Task[]>(key, (tasks) => tasks?.filter((t) => t.id !== taskId)),
  })
}

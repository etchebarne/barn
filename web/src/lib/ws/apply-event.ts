import type { QueryClient } from "@tanstack/react-query"

import type { Agent, WsEvent } from "../api-client"
import { addMessageToCache, applyActivityToAgents, markChatReadInCache } from "../chat-cache"
import { queryKeys } from "../query-keys"

/** Applies one realtime event to the TanStack Query cache (the single source of truth). */
export function applyWsEvent(queryClient: QueryClient, event: WsEvent): void {
  switch (event.type) {
    case "message.created":
      addMessageToCache(queryClient, event.message)
      return
    case "agent.activity":
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) =>
        agents ? applyActivityToAgents(agents, event.agentId, event.activity) : agents,
      )
      return
    case "chat.read":
      markChatReadInCache(queryClient, event.chatId, event.lastMessageId)
      return
  }
}

/** After a reconnect we may have missed events: refetch everything that's realtime-backed. */
export function resyncAfterReconnect(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => {
      const root = query.queryKey[0]
      return root === "chats" || root === "agents"
    },
  })
}

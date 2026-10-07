import type { QueryClient } from "@tanstack/react-query"

import type { Agent, Chat, WsEvent } from "../api-client"
import {
  addMessageToCache,
  applyActivityToAgents,
  markChatReadInCache,
  updateAgentInCache,
  updateMessageInCache,
  upsertAgent,
  upsertChat,
} from "../chat-cache"
import { queryKeys } from "../query-keys"

/** Applies one realtime event to the TanStack Query cache (the single source of truth). */
export function applyWsEvent(queryClient: QueryClient, event: WsEvent): void {
  switch (event.type) {
    case "message.created":
      addMessageToCache(queryClient, event.message)
      return
    case "message.updated":
      updateMessageInCache(queryClient, event.message)
      return
    case "agent.created":
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) =>
        agents ? upsertAgent(agents, event.agent) : agents,
      )
      return
    case "chat.created":
      queryClient.setQueryData<Chat[]>(queryKeys.chats, (chats) =>
        chats ? upsertChat(chats, event.chat) : chats,
      )
      return
    case "agent.activity":
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) =>
        agents ? applyActivityToAgents(agents, event.agentId, event.activity) : agents,
      )
      return
    case "agent.updated":
      updateAgentInCache(queryClient, event.agent)
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

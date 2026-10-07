import type { QueryClient } from "@tanstack/react-query"

import type { Agent, Chat, WsEvent } from "../api-client"
import {
  addMessageToCache,
  applyActivityToAgents,
  markChatReadInCache,
  removeArchivedAgent,
  removeDeletedAgent,
  updateAgentInCache,
  updateMessageInCache,
  upsertAgent,
  upsertChat,
} from "../chat-cache"
import { queryKeys } from "../query-keys"

export type ApplyResult = {
  /** Chats that no longer exist (e.g. an archived agent's DM); leave them if open. */
  removedChatIds: string[]
}

const NOTHING_REMOVED: ApplyResult = { removedChatIds: [] }

/** Applies one realtime event to the TanStack Query cache (the single source of truth). */
export function applyWsEvent(queryClient: QueryClient, event: WsEvent): ApplyResult {
  switch (event.type) {
    case "agent.deleted":
      return { removedChatIds: removeDeletedAgent(queryClient, event.agentId, event.chatId) }
    case "agent.archived":
      return { removedChatIds: removeArchivedAgent(queryClient, event.agentId) }
    case "message.created":
      addMessageToCache(queryClient, event.message)
      return NOTHING_REMOVED
    case "message.updated":
      updateMessageInCache(queryClient, event.message)
      return NOTHING_REMOVED
    case "agent.created":
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) =>
        agents ? upsertAgent(agents, event.agent) : agents,
      )
      return NOTHING_REMOVED
    case "chat.created":
      queryClient.setQueryData<Chat[]>(queryKeys.chats, (chats) =>
        chats ? upsertChat(chats, event.chat) : chats,
      )
      return NOTHING_REMOVED
    case "agent.activity":
      queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) =>
        agents ? applyActivityToAgents(agents, event.agentId, event.activity) : agents,
      )
      return NOTHING_REMOVED
    case "agent.updated":
      updateAgentInCache(queryClient, event.agent)
      return NOTHING_REMOVED
    case "chat.read":
      markChatReadInCache(queryClient, event.chatId, event.lastMessageId)
      return NOTHING_REMOVED
    default:
      // Events this client doesn't know yet.
      return NOTHING_REMOVED
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

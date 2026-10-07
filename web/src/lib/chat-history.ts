// Clearing a DM: the request, the cache update, and the hooks for UI-only state.
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, unwrap, type Chat } from "./api-client"
import { clearChatInCache } from "./chat-cache"
import { queryKeys } from "./query-keys"

type Listener = (chatId: string) => void

const listeners = new Set<Listener>()

/**
 * Subscribes to cleared chats, so UI-only state kept outside the query cache (pending sends,
 * a reply in progress) can be dropped too. Returns the unsubscribe function.
 */
export function onChatCleared(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** Applies a cleared history everywhere: the cache and every `onChatCleared` listener. */
export function applyChatCleared(queryClient: QueryClient, chatId: string) {
  clearChatInCache(queryClient, chatId)
  for (const listener of listeners) listener(chatId)
}

/**
 * Clears a DM's history: its messages and files go, and the agent forgets the conversation.
 * The cache is cleared on success too (not only on the `chat.cleared` event), and the toast
 * lives here so it still shows if the confirming UI unmounts.
 */
export function useClearChatHistory(chat: Pick<Chat, "id" | "name">) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      unwrap(api.DELETE("/chats/{chatId}/history", { params: { path: { chatId: chat.id } } })),
    onSuccess: () => {
      applyChatCleared(queryClient, chat.id)
      toast.success(`Cleared your DM with ${chat.name}`)
    },
    onError: (error) => toast.error(`Couldn't clear your DM with ${chat.name}: ${error.message}`),
  })
}

/** The confirmation's words, shared by every place that offers "Clear history". */
export function clearHistoryCopy(name: string) {
  return {
    title: `Clear your DM with ${name}?`,
    body: `This deletes every message and file in this chat, and ${name} forgets the conversation. Its saved memories, personality, tasks and group chats stay.`,
  }
}

/** The agent's DM from the chat list (the same query the sidebar uses), if it has one. */
export function useAgentDm(agentId: string): Chat | undefined {
  const { data } = useQuery({
    queryKey: queryKeys.chats,
    queryFn: () => unwrap(api.GET("/chats")),
    select: (chats) =>
      chats.find((chat) => chat.kind === "dm" && chat.members.some((m) => m.agentId === agentId)),
  })
  return data
}

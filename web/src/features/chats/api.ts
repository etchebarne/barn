import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query"

import { api, unwrap, type Message } from "@/lib/api-client"
import { addMessageToCache, markChatReadInCache } from "@/lib/chat-cache"
import { queryKeys } from "@/lib/query-keys"

import { pendingStore } from "./pending-store"

export const MESSAGE_PAGE_SIZE = 50

export const chatsQueryOptions = queryOptions({
  queryKey: queryKeys.chats,
  queryFn: () => unwrap(api.GET("/chats")),
})

export function messagesQueryOptions(chatId: string) {
  return infiniteQueryOptions({
    queryKey: queryKeys.messages(chatId),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/chats/{chatId}/messages", {
          params: { path: { chatId }, query: { before: pageParam, limit: MESSAGE_PAGE_SIZE } },
        }),
      ),
    initialPageParam: undefined as string | undefined,
    // Pages go newest → oldest; the next page is everything before the oldest loaded message.
    getNextPageParam: (oldestPage) => (oldestPage.hasMore ? oldestPage.messages[0]?.id : undefined),
    // Realtime keeps this fresh; refetches happen on reconnect.
    staleTime: Number.POSITIVE_INFINITY,
  })
}

/**
 * Sends a message. The text shows immediately as a pending bubble; when the server answers,
 * the real message goes into the cache (deduped by id against the WS `message.created` echo).
 */
export function useSendMessage(chatId: string) {
  const queryClient = useQueryClient()
  const { add, remove, confirm, fail, retrying } = pendingStore.getState()

  const mutation = useMutation({
    mutationFn: ({ clientId, body }: { clientId: string; body: string }) =>
      unwrap(
        api.POST("/chats/{chatId}/messages", {
          params: { path: { chatId } },
          // Echoed back on the response and the WS event, so the optimistic bubble is matched
          // exactly to the stored message.
          body: { body, clientId },
        }),
      ),
    onSuccess: (message: Message, { clientId }) => {
      // Keep the bubble (as "confirmed") until the message renders from the cache, so the row
      // is never briefly missing.
      confirm(chatId, clientId, message.id)
      addMessageToCache(queryClient, message)
    },
    onError: (_error, { clientId }) => fail(chatId, clientId),
  })

  return {
    send: (body: string) => {
      const clientId = crypto.randomUUID()
      add(chatId, { clientId, body, status: "sending", createdAt: Date.now() })
      mutation.mutate({ clientId, body })
    },
    retry: (clientId: string, body: string) => {
      retrying(chatId, clientId)
      mutation.mutate({ clientId, body })
    },
    discard: (clientId: string) => remove(chatId, clientId),
  }
}

export function useMarkRead(chatId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (lastMessageId: string) =>
      unwrap(
        api.POST("/chats/{chatId}/read", {
          params: { path: { chatId } },
          body: { lastMessageId },
        }),
      ),
    onMutate: (lastMessageId) => markChatReadInCache(queryClient, chatId, lastMessageId),
    onError: () => void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true }),
  })
}

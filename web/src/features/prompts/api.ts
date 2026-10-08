import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useCallback } from "react"

import { api, ApiError, unwrap, type Chat, type Message } from "@/lib/api-client"
import { findCachedMessage, updateMessageInCache } from "@/lib/chat-cache"
import { queryKeys } from "@/lib/query-keys"

import type { ConnectPromptRequest } from "./connect"
import { answerMessage, dismissMessage, type PromptAnswer } from "./logic"

type Snapshot = { previous: Message | undefined }

/** Optimistically applies `change`, then reconciles with the server's copy. */
function usePromptMutation<TVars>(
  message: Message,
  request: (vars: TVars) => Promise<Message>,
  change: (message: Message, vars: TVars) => Message,
) {
  const queryClient = useQueryClient()
  return useMutation<Message, Error, TVars, Snapshot>({
    mutationFn: request,
    onMutate: (vars) => {
      const previous = findCachedMessage(queryClient, message.chatId, message.id) ?? message
      updateMessageInCache(queryClient, change(previous, vars))
      return { previous }
    },
    onSuccess: (updated) => updateMessageInCache(queryClient, updated),
    onError: (error, _vars, snapshot) => {
      if (error instanceof ApiError && error.status === 409) {
        // Answered or dismissed elsewhere: take the server's version.
        void queryClient.invalidateQueries({ queryKey: queryKeys.messages(message.chatId) })
        return
      }
      if (snapshot?.previous) updateMessageInCache(queryClient, snapshot.previous)
    },
  })
}

export function useAnswerPrompt(message: Message) {
  return usePromptMutation<PromptAnswer>(
    message,
    (answer) =>
      unwrap(
        api.POST("/messages/{messageId}/answer", {
          params: { path: { messageId: message.id } },
          body: answer,
        }),
      ),
    answerMessage,
  )
}

export function useDismissPrompt(message: Message) {
  return usePromptMutation<void>(
    message,
    () =>
      unwrap(
        api.POST("/messages/{messageId}/dismiss", { params: { path: { messageId: message.id } } }),
      ),
    dismissMessage,
  )
}

/** Makes sure a chat is in the chat list before navigating to it (it may have just been created). */
export async function ensureChatLoaded(queryClient: QueryClient, chatId: string) {
  const chats = queryClient.getQueryData<Chat[]>(queryKeys.chats)
  if (chats?.some((chat) => chat.id === chatId)) return
  await queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true })
}

/**
 * Connects the app a "connect" prompt proposes. Deliberately not a `useMutation`: the request
 * carries secrets, and mutation state would keep them in the query client's cache.
 * Resolves on success (or a 409, after refetching); throws `ApiError` on 400/502 etc.
 */
export function useConnectPrompt(message: Message) {
  const queryClient = useQueryClient()
  return useCallback(
    async (body: ConnectPromptRequest) => {
      try {
        const updated = await unwrap(
          api.POST("/messages/{messageId}/connect", {
            params: { path: { messageId: message.id } },
            body,
          }),
        )
        updateMessageInCache(queryClient, updated)
        void queryClient.invalidateQueries({ queryKey: queryKeys.connectors })
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) {
          // Already answered elsewhere: show the server's version.
          void queryClient.invalidateQueries({ queryKey: queryKeys.messages(message.chatId) })
          return
        }
        throw error
      }
    },
    [message.id, message.chatId, queryClient],
  )
}

/**
 * Provides the secret a "secret" prompt asks for. Like connecting, deliberately not a
 * `useMutation` (mutation state would keep the value in the query client) and never
 * optimistic: only the server's updated message, which has no value, goes in the cache.
 */
export function useProvideSecret(message: Message) {
  const queryClient = useQueryClient()
  return useCallback(
    async (value: string) => {
      try {
        const updated = await unwrap(
          api.POST("/messages/{messageId}/secret", {
            params: { path: { messageId: message.id } },
            body: { value },
          }),
        )
        updateMessageInCache(queryClient, updated)
        void queryClient.invalidateQueries({
          queryKey: queryKeys.secrets(message.author.agentId ?? ""),
        })
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) {
          void queryClient.invalidateQueries({ queryKey: queryKeys.messages(message.chatId) })
          return
        }
        throw error
      }
    },
    [message.id, message.chatId, message.author.agentId, queryClient],
  )
}

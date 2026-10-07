import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query"

import { api, ApiError, unwrap, type Chat, type Message } from "@/lib/api-client"
import { findCachedMessage, updateMessageInCache } from "@/lib/chat-cache"
import { queryKeys } from "@/lib/query-keys"

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

import { createFileRoute } from "@tanstack/react-router"
import { useCallback, useMemo } from "react"

import { ChatView, messagesQueryOptions } from "@/features/chats"
import { readSignInReturn, useSignInReturn, type SignInReturnParams } from "@/features/connectors"

export const Route = createFileRoute("/_app/chats/$chatId")({
  // Connect cards come back here from sign-in: `?connected=<accountId>` or `?signin_error=…`.
  validateSearch: (search: Record<string, unknown>): SignInReturnParams => readSignInReturn(search),
  loader: ({ context, params }) => {
    void context.queryClient.prefetchInfiniteQuery(messagesQueryOptions(params.chatId))
  },
  component: ChatRoute,
})

function ChatRoute() {
  const { chatId } = Route.useParams()
  const navigate = Route.useNavigate()
  const { connected, signin_error } = Route.useSearch()
  const signInReturn = useMemo(() => ({ connected, signin_error }), [connected, signin_error])
  const clear = useCallback(() => void navigate({ search: {}, replace: true }), [navigate])
  useSignInReturn(signInReturn, clear)
  return <ChatView chatId={chatId} />
}

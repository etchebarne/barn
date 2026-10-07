import { createFileRoute } from "@tanstack/react-router"

import { ChatView, messagesQueryOptions } from "@/features/chats"

export const Route = createFileRoute("/_app/chats/$chatId")({
  loader: ({ context, params }) => {
    void context.queryClient.prefetchInfiniteQuery(messagesQueryOptions(params.chatId))
  },
  component: ChatRoute,
})

function ChatRoute() {
  const { chatId } = Route.useParams()
  return <ChatView chatId={chatId} />
}

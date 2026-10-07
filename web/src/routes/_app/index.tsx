import { createFileRoute, redirect } from "@tanstack/react-router"

import { chatsQueryOptions, ChatsEmptyState } from "@/features/chats"

export const Route = createFileRoute("/_app/")({
  loader: async ({ context }) => {
    const chats = await context.queryClient.ensureQueryData(chatsQueryOptions)
    const first = chats[0]
    if (first) throw redirect({ to: "/chats/$chatId", params: { chatId: first.id }, replace: true })
  },
  component: ChatsEmptyState,
})

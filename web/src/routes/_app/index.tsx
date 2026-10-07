import { createFileRoute, redirect } from "@tanstack/react-router"

import { chatsQueryOptions, ChatsEmptyState } from "@/features/chats"
import { useIsMobile } from "@/hooks/use-mobile"
import { firstChatRedirect, isMobileViewport } from "@/lib/mobile-nav"

export const Route = createFileRoute("/_app/")({
  loader: async ({ context }) => {
    const chats = await context.queryClient.ensureQueryData(chatsQueryOptions)
    // Desktop opens the first chat; on mobile `/` is the chat list (drawn by the app shell).
    const chatId = firstChatRedirect(isMobileViewport(), chats)
    if (chatId) throw redirect({ to: "/chats/$chatId", params: { chatId }, replace: true })
  },
  component: IndexRoute,
})

function IndexRoute() {
  const mobile = useIsMobile()
  return mobile ? null : <ChatsEmptyState />
}

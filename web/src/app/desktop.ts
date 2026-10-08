import { useQuery, type QueryClient } from "@tanstack/react-query"
import { useRouter } from "@tanstack/react-router"
import { useEffect } from "react"

import { chatsQueryOptions, plainPreview } from "@/features/chats"
import type { Agent, Chat, Message } from "@/lib/api-client"
import { openChatId } from "@/lib/chat-view"
import { desktop, desktopNotificationsEnabled } from "@/lib/desktop"
import { queryKeys } from "@/lib/query-keys"

export type DesktopNotification = { title: string; body: string; chatId: string; tag: string }

/**
 * The notification for a new message, or null when there shouldn't be one: only agents' messages
 * (not the user's or system events), only agents with notifications on, and not when the user
 * is looking at that chat (window focused and the chat open).
 */
export function desktopNotificationFor(
  message: Message,
  {
    agents,
    chats,
    focused,
    openChat,
  }: { agents: Agent[]; chats: Chat[]; focused: boolean; openChat: string | null },
): DesktopNotification | null {
  if (message.author.kind !== "agent" || message.event) return null
  const agent = agents.find((a) => a.id === message.author.agentId)
  if (!agent?.notifications) return null
  if (focused && openChat === message.chatId) return null
  const chat = chats.find((c) => c.id === message.chatId)
  const title = chat?.kind === "group" ? `${agent.name} in ${chat.name}` : agent.name
  const body = message.prompt
    ? "Asked you a question"
    : plainPreview(message.body, 200) ||
      (message.attachments.length > 1 ? `Sent ${message.attachments.length} files` : "Sent a file")
  return { title, body, chatId: message.chatId, tag: message.id }
}

/** Called for every live `message.created` (never for history or resyncs). */
export function notifyDesktop(queryClient: QueryClient, message: Message) {
  const bridge = desktop()
  if (!bridge || !desktopNotificationsEnabled()) return
  const notification = desktopNotificationFor(message, {
    agents: queryClient.getQueryData<Agent[]>(queryKeys.agents) ?? [],
    chats: queryClient.getQueryData<Chat[]>(queryKeys.chats) ?? [],
    focused: document.hasFocus(),
    openChat: openChatId(),
  })
  if (notification) bridge.notify(notification)
}

/** Total unread across chats. */
export function unreadTotal(chats: Chat[]): number {
  return chats.reduce((sum, chat) => sum + chat.unreadCount, 0)
}

/**
 * Desktop app glue: follows the app's navigation requests (e.g. a clicked notification) and
 * keeps its badge in sync with the unread total. Does nothing in a browser.
 */
export function useDesktopIntegration() {
  const router = useRouter()
  const bridge = desktop()
  const { data: total } = useQuery({
    ...chatsQueryOptions,
    select: unreadTotal,
    enabled: bridge !== null,
  })

  useEffect(() => {
    if (!bridge) return undefined
    return bridge.onNavigate((path) => {
      if (path.startsWith("/")) router.history.push(path)
    })
  }, [bridge, router])

  useEffect(() => {
    if (bridge && total !== undefined) bridge.setUnread(total)
  }, [bridge, total])
}

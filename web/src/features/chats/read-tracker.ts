import { useEffect, useRef, useSyncExternalStore } from "react"

import { useMessageScrollerVisibility } from "@/components/ui/message-scroller"
import type { Message } from "@/lib/api-client"
import { setChatView } from "@/lib/chat-view"

import { useMarkRead } from "./api"

function subscribeVisibility(onChange: () => void) {
  document.addEventListener("visibilitychange", onChange)
  return () => document.removeEventListener("visibilitychange", onChange)
}

function usePageVisible() {
  return useSyncExternalStore(
    subscribeVisibility,
    () => document.visibilityState === "visible",
    () => true,
  )
}

/**
 * Marks the chat read when its latest message is on screen and the tab is visible, and tells
 * the realtime layer whether this chat is in view (so new messages don't count as unread).
 * Must be rendered inside the chat's MessageScrollerProvider.
 */
export function useMarkReadWhenSeen({
  chatId,
  loaded,
  latest,
  latestRowId,
  unread,
}: {
  chatId: string
  /** The first page of messages has loaded. */
  loaded: boolean
  latest: Message | undefined
  /** The id of the last row in the scroller (may differ from the latest message's id). */
  latestRowId: string | undefined
  unread: number
}) {
  const { visibleMessageIds } = useMessageScrollerVisibility()
  const pageVisible = usePageVisible()
  const { mutate: markRead } = useMarkRead(chatId)
  const lastMarked = useRef<string | null>(null)
  const seeded = useRef(false)
  const latestVisible = latestRowId !== undefined && visibleMessageIds.includes(latestRowId)

  useEffect(() => {
    setChatView({ chatId, atLatest: latestVisible })
    return () => setChatView(null)
  }, [chatId, latestVisible])

  const latestId = latest?.id

  // Messages already read when the chat opened don't need a read receipt.
  useEffect(() => {
    if (!loaded || seeded.current) return
    seeded.current = true
    if (unread === 0 && latestId) lastMarked.current = latestId
  }, [loaded, latestId, unread])
  // A message that arrived while in view wasn't counted as unread, but the server still needs
  // the read receipt. The user's own messages never need one.
  const needsReceipt = unread > 0 || (latest !== undefined && latest.author.kind !== "user")

  useEffect(() => {
    if (!seeded.current || !latestId || !latestVisible || !pageVisible || !needsReceipt) return
    if (lastMarked.current === latestId) return
    lastMarked.current = latestId
    markRead(latestId)
  }, [latestId, latestVisible, pageVisible, needsReceipt, markRead])
}

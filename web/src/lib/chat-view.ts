/**
 * Which chat the user is looking at, and whether its latest message is on screen. The realtime
 * layer uses this to avoid counting messages as unread when the user is already reading them
 * (otherwise the sidebar flashes a "1" until mark-read lands).
 */
type ChatView = { chatId: string; atLatest: boolean }

let current: ChatView | null = null

export function setChatView(view: ChatView | null) {
  current = view
}

/** True when `chatId` is open, scrolled to its latest message, and the tab is visible. */
export function isChatInView(chatId: string): boolean {
  return (
    current !== null &&
    current.chatId === chatId &&
    current.atLatest &&
    document.visibilityState === "visible"
  )
}

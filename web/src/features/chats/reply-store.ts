import { create } from "zustand"

import type { Message, Schemas } from "@/lib/api-client"
import { isImage } from "@/lib/attachments"
import { onChatCleared } from "@/lib/chat-history"

import { plainPreview } from "./preview"

export type QuotedMessage = Schemas["QuotedMessage"]

type ReplyState = {
  /** The message being replied to, per chat. */
  byChat: Record<string, Message | null>
  /** Bumped to ask the composer to take focus. */
  focusRequest: number
  startReply: (message: Message) => void
  cancelReply: (chatId: string) => void
  /** Ask the open chat's composer to take focus (e.g. after starting a draft for the user). */
  requestFocus: () => void
}

export const useReplyStore = create<ReplyState>()((set) => ({
  byChat: {},
  focusRequest: 0,
  startReply: (message) =>
    set((s) => ({
      byChat: { ...s.byChat, [message.chatId]: message },
      focusRequest: s.focusRequest + 1,
    })),
  cancelReply: (chatId) => set((s) => ({ byChat: { ...s.byChat, [chatId]: null } })),
  requestFocus: () => set((s) => ({ focusRequest: s.focusRequest + 1 })),
}))

// A cleared chat has nothing left to reply to.
onChatCleared((chatId) => useReplyStore.getState().cancelReply(chatId))

export function useReplyTo(chatId: string): Message | null {
  return useReplyStore((s) => s.byChat[chatId] ?? null)
}

const QUOTE_MAX = 300

/** The quote the server would attach, built from a cached message for the pending bubble. */
export function quoteOf(message: Message): QuotedMessage {
  const body =
    message.body.length > QUOTE_MAX ? `${message.body.slice(0, QUOTE_MAX)}…` : message.body
  return {
    id: message.id,
    available: true,
    author: message.author,
    body,
    attachments: message.attachments,
  }
}

/**
 * One line describing a quoted message: its text without markdown, else "Image" or the first
 * file's name (with how many other attachments there are).
 */
export function quoteExcerpt(
  quote: Pick<QuotedMessage, "body" | "attachments">,
  max = 160,
): string {
  const text = plainPreview(quote.body ?? "", max)
  if (text) return text
  const attachments = quote.attachments ?? []
  if (attachments.length === 0) return ""
  if (attachments.every((a) => isImage(a.mime))) {
    return attachments.length === 1 ? "Image" : `${attachments.length} images`
  }
  const name = attachments.find((a) => !isImage(a.mime))?.name ?? "File"
  return attachments.length > 1 ? `${name} +${attachments.length - 1}` : name
}

import { useStore } from "zustand"
import { createStore } from "zustand/vanilla"

import type { Message } from "@/lib/api-client"

export type PendingMessage = {
  clientId: string
  body: string
  /**
   * "confirmed": the server accepted it as `messageId`. It's hidden once that message is cached,
   * but kept for the session so the delivered message keeps this entry's row key.
   */
  status: "sending" | "failed" | "confirmed"
  createdAt: number
  messageId?: string
}

type PendingState = {
  byChat: Record<string, PendingMessage[]>
  add: (chatId: string, message: PendingMessage) => void
  remove: (chatId: string, clientId: string) => void
  confirm: (chatId: string, clientId: string, messageId: string) => void
  fail: (chatId: string, clientId: string) => void
  retrying: (chatId: string, clientId: string) => void
}

function update(
  byChat: Record<string, PendingMessage[]>,
  chatId: string,
  fn: (list: PendingMessage[]) => PendingMessage[],
) {
  return { byChat: { ...byChat, [chatId]: fn(byChat[chatId] ?? []) } }
}

function setStatus(status: PendingMessage["status"], clientId: string) {
  return (list: PendingMessage[]) =>
    list.map((m) => (m.clientId === clientId ? { ...m, status } : m))
}

/** Messages the user sent that the server hasn't confirmed yet (UI-only state). */
export const pendingStore = createStore<PendingState>()((set) => ({
  byChat: {},
  add: (chatId, message) => set((s) => update(s.byChat, chatId, (list) => [...list, message])),
  remove: (chatId, clientId) =>
    set((s) => update(s.byChat, chatId, (list) => list.filter((m) => m.clientId !== clientId))),
  confirm: (chatId, clientId, messageId) =>
    set((s) =>
      update(s.byChat, chatId, (list) =>
        list.map((m) => (m.clientId === clientId ? { ...m, status: "confirmed", messageId } : m)),
      ),
    ),
  fail: (chatId, clientId) => set((s) => update(s.byChat, chatId, setStatus("failed", clientId))),
  retrying: (chatId, clientId) =>
    set((s) => update(s.byChat, chatId, setStatus("sending", clientId))),
}))

const EMPTY: PendingMessage[] = []

export function usePendingMessages(chatId: string): PendingMessage[] {
  return useStore(pendingStore, (s) => s.byChat[chatId] ?? EMPTY)
}

export type Reconciled = {
  /** Pending bubbles still to show. */
  visible: PendingMessage[]
  /**
   * Delivered message id → the client id its pending bubble used. Delivered messages render
   * under that key, so the scroller sees one stable row instead of a remove + insert.
   */
  rowKeys: Map<string, string>
}

/**
 * Matches pending bubbles to delivered messages by `clientId` (echoed by the server on the
 * POST response and the `message.created` event), or by id once the POST confirmed them, which
 * survives refetches that drop `clientId`. Matched bubbles are hidden, so the user never sees
 * their message twice.
 */
export function reconcilePending(pending: PendingMessage[], messages: Message[]): Reconciled {
  const rowKeys = new Map<string, string>()
  if (pending.length === 0) return { visible: pending, rowKeys }
  const byClientId = new Map<string, Message>()
  const byId = new Map<string, Message>()
  for (const m of messages) {
    byId.set(m.id, m)
    if (m.clientId) byClientId.set(m.clientId, m)
  }
  const visible = pending.filter((p) => {
    const match = byClientId.get(p.clientId) ?? (p.messageId ? byId.get(p.messageId) : undefined)
    if (!match) return true
    rowKeys.set(match.id, p.clientId)
    return false
  })
  return { visible, rowKeys }
}

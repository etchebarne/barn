import { QueryClient } from "@tanstack/react-query"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { makeAgent, makeChat, makeMessage } from "@/test/fixtures"

import type { Agent, Chat } from "../api-client"
import { flattenMessages, type MessagesData } from "../chat-cache"
import { setChatView } from "../chat-view"
import { queryKeys } from "../query-keys"
import { applyWsEvent, resyncAfterReconnect } from "./apply-event"

function seedMessages(qc: QueryClient, chatId: string, data: MessagesData) {
  qc.setQueryData(queryKeys.messages(chatId), data)
}

function firstUnread(qc: QueryClient) {
  return qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.unreadCount
}

describe("applyWsEvent", () => {
  let qc: QueryClient

  beforeEach(() => {
    qc = new QueryClient()
  })

  describe("message.created", () => {
    it("appends to the newest page of the chat's message cache", () => {
      const first = makeMessage({ body: "first" })
      seedMessages(qc, "chat-1", {
        pages: [{ messages: [first], hasMore: false }],
        pageParams: [undefined],
      })
      const incoming = makeMessage({ body: "second" })

      applyWsEvent(qc, { type: "message.created", message: incoming })

      const data = qc.getQueryData<MessagesData>(queryKeys.messages("chat-1"))
      expect(flattenMessages(data).map((m) => m.body)).toEqual(["first", "second"])
    })

    it("ignores a message that's already cached (optimistic send + WS echo)", () => {
      const message = makeMessage({ author: { kind: "user", agentId: null } })
      seedMessages(qc, "chat-1", {
        pages: [{ messages: [message], hasMore: false }],
        pageParams: [undefined],
      })
      qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat({ lastMessage: message })])

      applyWsEvent(qc, { type: "message.created", message })

      const data = qc.getQueryData<MessagesData>(queryKeys.messages("chat-1"))
      expect(flattenMessages(data)).toHaveLength(1)
      expect(qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.unreadCount).toBe(0)
    })

    it("doesn't create a message cache for chats that were never opened", () => {
      applyWsEvent(qc, { type: "message.created", message: makeMessage({ chatId: "other" }) })
      expect(qc.getQueryData(queryKeys.messages("other"))).toBeUndefined()
    })

    it("updates last message, unread count and ordering in the chat list", () => {
      const older = makeMessage({ chatId: "a" })
      const chatA = makeChat({ id: "a", lastMessage: older })
      const chatB = makeChat({ id: "b", lastMessage: makeMessage({ chatId: "b" }) })
      qc.setQueryData<Chat[]>(queryKeys.chats, [chatB, chatA])

      const incoming = makeMessage({ chatId: "a", body: "ping" })
      applyWsEvent(qc, { type: "message.created", message: incoming })

      const chats = qc.getQueryData<Chat[]>(queryKeys.chats) ?? []
      expect(chats.map((c) => c.id)).toEqual(["a", "b"])
      expect(chats[0]?.lastMessage?.body).toBe("ping")
      expect(chats[0]?.unreadCount).toBe(1)
    })

    describe("when the chat is open", () => {
      let visibility: DocumentVisibilityState

      beforeEach(() => {
        visibility = "visible"
        vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility)
        qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat({ unreadCount: 0 })])
      })

      afterEach(() => setChatView(null))

      it("doesn't count a reply as unread while the latest message is on screen", () => {
        setChatView({ chatId: "chat-1", atLatest: true })
        applyWsEvent(qc, { type: "message.created", message: makeMessage({ body: "done" }) })
        expect(firstUnread(qc)).toBe(0)
        expect(qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.lastMessage?.body).toBe("done")
      })

      it("counts it when scrolled up", () => {
        setChatView({ chatId: "chat-1", atLatest: false })
        applyWsEvent(qc, { type: "message.created", message: makeMessage() })
        expect(firstUnread(qc)).toBe(1)
      })

      it("counts it when the tab is hidden", () => {
        setChatView({ chatId: "chat-1", atLatest: true })
        visibility = "hidden"
        applyWsEvent(qc, { type: "message.created", message: makeMessage() })
        expect(firstUnread(qc)).toBe(1)
      })

      it("counts messages for other chats", () => {
        setChatView({ chatId: "other", atLatest: true })
        applyWsEvent(qc, { type: "message.created", message: makeMessage() })
        expect(firstUnread(qc)).toBe(1)
      })
    })

    it("doesn't count the user's own messages as unread", () => {
      qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat({ unreadCount: 0 })])
      applyWsEvent(qc, {
        type: "message.created",
        message: makeMessage({ author: { kind: "user", agentId: null } }),
      })
      expect(qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.unreadCount).toBe(0)
    })

    it("refetches the chat list when the message belongs to an unknown chat", () => {
      qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat()])
      const spy = vi.spyOn(qc, "invalidateQueries")
      applyWsEvent(qc, { type: "message.created", message: makeMessage({ chatId: "new" }) })
      expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.chats, exact: true })
    })
  })

  describe("agent.activity", () => {
    it("updates the agent's activity in the agents list", () => {
      qc.setQueryData<Agent[]>(queryKeys.agents, [makeAgent(), makeAgent({ id: "agent-2" })])

      applyWsEvent(qc, {
        type: "agent.activity",
        agentId: "agent-2",
        activity: { state: "working", label: "running npm test" },
      })

      const agents = qc.getQueryData<Agent[]>(queryKeys.agents) ?? []
      expect(agents[0]?.activity.state).toBe("idle")
      expect(agents[1]?.activity).toEqual({ state: "working", label: "running npm test" })
    })
  })

  describe("chat.read", () => {
    it("clears the unread count when read up to the last message", () => {
      const last = makeMessage()
      qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat({ lastMessage: last, unreadCount: 3 })])

      applyWsEvent(qc, { type: "chat.read", chatId: "chat-1", lastMessageId: last.id })

      expect(qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.unreadCount).toBe(0)
    })

    it("refetches instead of guessing when newer messages exist", () => {
      const read = makeMessage()
      const newer = makeMessage()
      qc.setQueryData<Chat[]>(queryKeys.chats, [makeChat({ lastMessage: newer, unreadCount: 2 })])
      const spy = vi.spyOn(qc, "invalidateQueries")

      applyWsEvent(qc, { type: "chat.read", chatId: "chat-1", lastMessageId: read.id })

      expect(qc.getQueryData<Chat[]>(queryKeys.chats)?.[0]?.unreadCount).toBe(2)
      expect(spy).toHaveBeenCalledWith({ queryKey: queryKeys.chats, exact: true })
    })
  })
})

describe("resyncAfterReconnect", () => {
  it("invalidates chat and agent queries but leaves auth alone", async () => {
    const qc = new QueryClient()
    qc.setQueryData(queryKeys.authStatus, { setupRequired: false, user: null })
    qc.setQueryData(queryKeys.chats, [])
    qc.setQueryData(queryKeys.agents, [])
    qc.setQueryData(queryKeys.messages("chat-1"), { pages: [], pageParams: [] })

    await resyncAfterReconnect(qc)

    expect(qc.getQueryState(queryKeys.chats)?.isInvalidated).toBe(true)
    expect(qc.getQueryState(queryKeys.agents)?.isInvalidated).toBe(true)
    expect(qc.getQueryState(queryKeys.messages("chat-1"))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(queryKeys.authStatus)?.isInvalidated).toBe(false)
  })
})

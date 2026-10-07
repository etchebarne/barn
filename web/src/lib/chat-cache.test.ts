import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import { flattenMessages, insertMessage, type MessagesData } from "./chat-cache"

describe("flattenMessages", () => {
  it("orders pages oldest-first and drops duplicates across page boundaries", () => {
    const [a, b, c] = [makeMessage(), makeMessage(), makeMessage()]
    if (!a || !b || !c) throw new Error("fixtures")
    const data: MessagesData = {
      // pages[0] is the newest page; pages[1] is older history.
      pages: [
        { messages: [b, c], hasMore: true },
        { messages: [a, b], hasMore: false },
      ],
      pageParams: [undefined, b.id],
    }
    expect(flattenMessages(data).map((m) => m.id)).toEqual([a.id, b.id, c.id])
  })
})

describe("insertMessage", () => {
  it("keeps chronological order when a message arrives out of order", () => {
    const early = makeMessage()
    const late = makeMessage()
    const data: MessagesData = {
      pages: [{ messages: [late], hasMore: false }],
      pageParams: [undefined],
    }
    const next = insertMessage(data, early)
    expect(next.pages[0]?.messages.map((m) => m.id)).toEqual([early.id, late.id])
  })

  it("returns the same object when the message is already present", () => {
    const m = makeMessage()
    const data: MessagesData = {
      pages: [{ messages: [m], hasMore: false }],
      pageParams: [undefined],
    }
    expect(insertMessage(data, m)).toBe(data)
  })
})

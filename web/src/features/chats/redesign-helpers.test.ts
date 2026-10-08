import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import { formatElapsed } from "./activity-line"
import { formatListTime } from "./grouping"
import { firstUnreadId } from "./message-list"
import { isRichBody } from "./message-row"

describe("formatListTime", () => {
  const now = new Date(2026, 9, 8, 15, 0) // Thursday
  it("shows the time today, then Yesterday, a weekday, a date", () => {
    expect(formatListTime(new Date(2026, 9, 8, 9, 5).toISOString(), now)).toMatch(/9:05/)
    expect(formatListTime(new Date(2026, 9, 7, 23, 0).toISOString(), now)).toBe("Yesterday")
    expect(formatListTime(new Date(2026, 9, 4, 12, 0).toISOString(), now)).toMatch(/Sun/)
    expect(formatListTime(new Date(2026, 8, 20, 12, 0).toISOString(), now)).toMatch(/Sep/)
  })
})

const agent = (id: string) => makeMessage({ id, author: { kind: "agent", agentId: "a" } })
const user = (id: string) => makeMessage({ id, author: { kind: "user", agentId: null } })

describe("firstUnreadId", () => {
  it("walks back past the unread count, skipping the user's own messages", () => {
    const messages = [agent("1"), agent("2"), user("3"), agent("4")]
    expect(firstUnreadId(messages, 2)).toBe("2")
    expect(firstUnreadId(messages, 1)).toBe("4")
  })

  it("is null when nothing is unread or it's not loaded", () => {
    expect(firstUnreadId([agent("1")], 0)).toBeNull()
    expect(firstUnreadId([agent("1")], 3)).toBeNull()
  })
})

describe("isRichBody", () => {
  it("is true for code, tables and headings", () => {
    expect(isRichBody("Here:\n\n```go\nx := 1\n```")).toBe(true)
    expect(isRichBody("| a | b |\n|---|---|\n| 1 | 2 |")).toBe(true)
    expect(isRichBody("## Summary\n\nAll good.")).toBe(true)
  })

  it("is false for prose, lists and inline code", () => {
    expect(isRichBody("Sure, I'll do it. Use `npm test`.")).toBe(false)
    expect(isRichBody("1. one\n2. two")).toBe(false)
    expect(isRichBody("Price is #1 | maybe")).toBe(false)
  })
})

describe("formatElapsed", () => {
  it("counts seconds, then minutes and padded seconds", () => {
    expect(formatElapsed(7)).toBe("7s")
    expect(formatElapsed(65)).toBe("1m 05s")
  })
})

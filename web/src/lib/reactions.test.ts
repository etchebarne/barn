import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import { toggleUserReaction, userReacted } from "./reactions"

const you = { kind: "user", agentId: null } as const
const barn = { kind: "agent", agentId: "a1" } as const

describe("toggleUserReaction", () => {
  it("adds a new emoji at the end", () => {
    const message = makeMessage({ reactions: [{ emoji: "👍", by: [barn] }] })
    const next = toggleUserReaction(message, "🎉")
    expect(next.reactions).toEqual([
      { emoji: "👍", by: [barn] },
      { emoji: "🎉", by: [you] },
    ])
    expect(message.reactions).toHaveLength(1)
  })

  it("joins an existing emoji someone else used", () => {
    const next = toggleUserReaction(makeMessage({ reactions: [{ emoji: "👍", by: [barn] }] }), "👍")
    expect(next.reactions).toEqual([{ emoji: "👍", by: [barn, you] }])
    expect(next.reactions.every(userReacted)).toBe(true)
  })

  it("removes your reaction, and the emoji once nobody's left", () => {
    const shared = makeMessage({ reactions: [{ emoji: "👍", by: [barn, you] }] })
    expect(toggleUserReaction(shared, "👍").reactions).toEqual([{ emoji: "👍", by: [barn] }])

    const onlyYou = makeMessage({ reactions: [{ emoji: "👀", by: [you] }] })
    expect(toggleUserReaction(onlyYou, "👀").reactions).toEqual([])
  })
})

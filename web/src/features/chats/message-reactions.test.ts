import { describe, expect, it } from "vitest"

import { makeAgent } from "@/test/fixtures"

import { reactorNames } from "./message-reactions"

const agent = (id: string) => ({ kind: "agent" as const, agentId: id })

describe("reactorNames", () => {
  const agents = new Map([
    ["a1", makeAgent({ id: "a1", name: "barn" })],
    ["a2", makeAgent({ id: "a2", name: "Tracker" })],
  ])

  it("names one, two, or several reactors", () => {
    expect(reactorNames({ emoji: "👍", by: [agent("a1")] }, agents)).toBe("barn")
    expect(reactorNames({ emoji: "👍", by: [agent("a1"), agent("a2")] }, agents)).toBe(
      "barn and Tracker",
    )
    expect(
      reactorNames(
        { emoji: "👍", by: [agent("a1"), agent("a2"), { kind: "user", agentId: null }] },
        agents,
      ),
    ).toBe("barn, Tracker and you")
  })

  it("falls back when an agent isn't loaded", () => {
    expect(reactorNames({ emoji: "👀", by: [agent("gone")] }, agents)).toBe("An agent")
  })
})

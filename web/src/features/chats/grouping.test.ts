import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import { toRows } from "./grouping"

const at = (minute: number) => `2026-10-06T12:${String(minute).padStart(2, "0")}:00.000Z`
const agent = (minute: number, agentId = "agent-1") =>
  makeMessage({ author: { kind: "agent", agentId }, createdAt: at(minute) })
const user = (minute: number) =>
  makeMessage({ author: { kind: "user", agentId: null }, createdAt: at(minute) })
const system = (minute: number) =>
  makeMessage({ author: { kind: "system", agentId: null }, createdAt: at(minute) })

const shape = (rows: ReturnType<typeof toRows>) =>
  rows.map((r) => `${r.startsRun ? "[" : ""}${r.message.author.kind[0]}${r.endsRun ? "]" : ""}`)

describe("toRows", () => {
  it("groups consecutive messages from the same author (avatar on the last)", () => {
    expect(shape(toRows([agent(0), agent(0), agent(1), user(2), agent(3)]))).toEqual([
      "[a",
      "a",
      "a]",
      "[u]",
      "[a]",
    ])
  })

  it("breaks groups on a different agent, a system message, or a long pause", () => {
    expect(shape(toRows([agent(0), agent(0, "agent-2")]))).toEqual(["[a]", "[a]"])
    expect(shape(toRows([agent(0), system(0), agent(0)]))).toEqual(["[a]", "[s]", "[a]"])
    expect(shape(toRows([agent(0), agent(10)]))).toEqual(["[a]", "[a]"])
  })

  it("marks the first message of each day", () => {
    const rows = toRows([
      makeMessage({ createdAt: "2026-10-05T12:00:00.000Z" }),
      makeMessage({ createdAt: "2026-10-06T12:00:00.000Z" }),
      makeMessage({ createdAt: "2026-10-06T12:01:00.000Z" }),
    ])
    expect(rows.map((r) => r.startsDay)).toEqual([true, true, false])
  })
})

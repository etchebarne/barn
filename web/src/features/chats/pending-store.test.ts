import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import { reconcilePending, type PendingMessage } from "./pending-store"

function pending(clientId: string, overrides: Partial<PendingMessage> = {}): PendingMessage {
  return { clientId, body: "hi", status: "sending", createdAt: 0, ...overrides }
}

const mine = { kind: "user", agentId: null } as const

describe("reconcilePending", () => {
  it("hides a pending bubble once a message with its clientId is delivered (WS echo first)", () => {
    const echoed = makeMessage({ author: mine, clientId: "c-1" })
    const { visible, rowKeys } = reconcilePending([pending("c-1"), pending("c-2")], [echoed])
    expect(visible.map((p) => p.clientId)).toEqual(["c-2"])
    // The delivered message takes over the pending bubble's row key.
    expect(rowKeys.get(echoed.id)).toBe("c-1")
  })

  it("never matches by text: an identical message without our clientId stays separate", () => {
    const sameText = makeMessage({ author: mine, body: "hi" })
    expect(reconcilePending([pending("c-1")], [sameText]).visible).toHaveLength(1)
  })

  it("hides a failed bubble if the message did get through", () => {
    const delivered = makeMessage({ author: mine, clientId: "c-1" })
    expect(reconcilePending([pending("c-1", { status: "failed" })], [delivered]).visible).toEqual(
      [],
    )
  })

  it("keeps the row key by id after a refetch drops clientId", () => {
    const confirmed = pending("c-1", { status: "confirmed", messageId: "m-1" })
    expect(reconcilePending([confirmed], []).visible).toEqual([confirmed])

    const refetched = makeMessage({ id: "m-1", author: mine })
    const result = reconcilePending([confirmed], [refetched])
    expect(result.visible).toEqual([])
    expect(result.rowKeys.get("m-1")).toBe("c-1")
  })
})

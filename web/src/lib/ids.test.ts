import { afterEach, describe, expect, it, vi } from "vitest"

import { randomId } from "./ids"

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

describe("randomId", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("works without crypto.randomUUID (plain http)", () => {
    const real = globalThis.crypto
    vi.stubGlobal("crypto", {
      getRandomValues: (a: Uint8Array<ArrayBuffer>) => real.getRandomValues(a),
    })
    const ids = new Set(Array.from({ length: 50 }, randomId))
    expect(ids.size).toBe(50)
    for (const id of ids) expect(id).toMatch(UUID)
  })

  it("uses crypto.randomUUID when available", () => {
    expect(randomId()).toMatch(UUID)
  })
})

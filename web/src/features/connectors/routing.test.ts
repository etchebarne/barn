import { describe, expect, it } from "vitest"

import { connectorsForward, readConnectorsSearch } from "./signin"

function forward(search: Record<string, string>) {
  return connectorsForward(readConnectorsSearch(search))
}

describe("connectors routing", () => {
  it("forwards /settings?connector= (and OAuth returns) to /connectors with the same params", () => {
    expect(forward({ connector: "k1" })).toEqual({ connector: "k1" })
    expect(forward({ connector: "k1", connected: "1" })).toEqual({
      connector: "k1",
      connected: "1",
    })
    expect(forward({ signin_error: "Access denied" })).toEqual({ signin_error: "Access denied" })
    expect(forward({})).toBeNull()
    expect(forward({ unrelated: "x" })).toBeNull()
  })

  it("keeps the open connection and sign-in params on /connectors", () => {
    expect(readConnectorsSearch({ connector: "new", connected: "k2", other: "x" })).toEqual({
      connector: "new",
      connected: "k2",
    })
  })
})

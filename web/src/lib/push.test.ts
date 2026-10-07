import { describe, expect, it } from "vitest"

import { pushState, urlBase64ToUint8Array } from "./push"

const base = {
  supported: true,
  serverAvailable: true,
  permission: "default",
  subscribed: false,
} as const

describe("pushState", () => {
  it("is unsupported without the Push API, whatever else is true", () => {
    expect(pushState({ ...base, supported: false, subscribed: true })).toBe("unsupported")
  })

  it("is unavailable when the server has no push key", () => {
    expect(pushState({ ...base, serverAvailable: false })).toBe("unavailable")
  })

  it("is blocked when permission was denied", () => {
    expect(pushState({ ...base, permission: "denied" })).toBe("blocked")
    expect(pushState({ ...base, permission: "denied", subscribed: true })).toBe("blocked")
  })

  it("is on only with permission and a subscription", () => {
    expect(pushState({ ...base, permission: "granted", subscribed: true })).toBe("on")
    expect(pushState({ ...base, permission: "granted", subscribed: false })).toBe("off")
    expect(pushState({ ...base, permission: "default", subscribed: false })).toBe("off")
  })
})

describe("urlBase64ToUint8Array", () => {
  it("decodes base64url without padding", () => {
    expect([...urlBase64ToUint8Array("AQID-_8")]).toEqual([1, 2, 3, 251, 255])
  })
})

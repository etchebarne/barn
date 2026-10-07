import { describe, expect, it } from "vitest"

import { isAutoLanguage, trustChangeNeedsConfirmation } from "./trust"

describe("trustChangeNeedsConfirmation", () => {
  it("confirms only when turning trusted mode on", () => {
    expect(trustChangeNeedsConfirmation("ask", "trusted")).toBe(true)
    expect(trustChangeNeedsConfirmation("trusted", "ask")).toBe(false)
    expect(trustChangeNeedsConfirmation("trusted", "trusted")).toBe(false)
    expect(trustChangeNeedsConfirmation("ask", "ask")).toBe(false)
  })
})

describe("isAutoLanguage", () => {
  it("treats 'auto' case-insensitively", () => {
    expect(isAutoLanguage("auto")).toBe(true)
    expect(isAutoLanguage(" Auto ")).toBe(true)
    expect(isAutoLanguage("Spanish")).toBe(false)
  })
})

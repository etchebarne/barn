import { describe, expect, it } from "vitest"

import { validateSetup } from "./validation"

describe("validateSetup", () => {
  it("requires a username, a 12+ character password and a matching confirmation", () => {
    expect(validateSetup({ username: "", password: "short", confirm: "" })).toEqual({
      username: "Choose a username.",
      password: "Use at least 12 characters.",
    })
    expect(
      validateSetup({ username: "martin", password: "correct horse", confirm: "correct house" }),
    ).toEqual({ confirm: "Passwords don't match." })
    expect(
      validateSetup({ username: "martin", password: "correct horse", confirm: "correct horse" }),
    ).toEqual({})
  })
})

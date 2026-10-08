import { describe, expect, it } from "vitest"

import { slugify } from "./api"

describe("slugify", () => {
  it.each([
    ["Weekly Report!", "weekly-report"],
    ["  Résumé  helper ", "resume-helper"],
    ["--x--", "x"],
    ["!!!", ""],
  ])("%s → %s", (text, want) => expect(slugify(text)).toBe(want))

  it("stays within 64 characters without a trailing hyphen", () => {
    const name = slugify(`${"a".repeat(63)} b`)
    expect(name).toBe("a".repeat(63))
  })
})

import { describe, expect, it } from "vitest"

import { splitSnippet } from "./search"

describe("splitSnippet", () => {
  it("marks the matched words", () => {
    expect(splitSnippet("…the \u0002deploy\u0003 of \u0002café\u0003-api failed")).toEqual([
      { text: "…the ", match: false, start: 0 },
      { text: "deploy", match: true, start: 5 },
      { text: " of ", match: false, start: 11 },
      { text: "café", match: true, start: 15 },
      { text: "-api failed", match: false, start: 19 },
    ])
  })

  it("drops Markdown emphasis and code marks", () => {
    expect(
      splitSnippet("**Hi**, try `/table`")
        .map((p) => p.text)
        .join(""),
    ).toBe("Hi, try /table")
  })

  it("keeps text without matches whole", () => {
    expect(splitSnippet("plain")).toEqual([{ text: "plain", match: false, start: 0 }])
  })
})

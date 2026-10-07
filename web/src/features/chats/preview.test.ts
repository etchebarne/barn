import { describe, expect, it } from "vitest"

import { makeAgent, makeChat, makeMessage } from "@/test/fixtures"

import { chatPreview, plainPreview } from "./preview"

describe("plainPreview", () => {
  it("strips markdown to a single line", () => {
    expect(plainPreview("## Done\n\n- **tests** pass\n- see [PR](https://x.y)")).toBe(
      "Done tests pass see PR",
    )
    expect(plainPreview("run:\n```sh\nnpm test\n```")).toBe("run: [code]")
  })
})

describe("chatPreview", () => {
  const agents = new Map([["agent-1", makeAgent({ name: "scout" })]])

  it("prefixes your own messages and agent names in groups", () => {
    const mine = makeMessage({ author: { kind: "user", agentId: null }, body: "hey" })
    expect(chatPreview(makeChat({ lastMessage: mine }), agents)).toBe("You: hey")

    const theirs = makeMessage({ body: "on it" })
    expect(chatPreview(makeChat({ lastMessage: theirs }), agents)).toBe("on it")
    expect(chatPreview(makeChat({ kind: "group", lastMessage: theirs }), agents)).toBe(
      "scout: on it",
    )
  })
})

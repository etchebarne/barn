import { describe, expect, it } from "vitest"

import {
  activeMentionQuery,
  filterMentionCandidates,
  insertMention,
  mentionTargets,
  splitMentions,
} from "./mentions"

const alpha = { id: "a", name: "Alpha" }
const opsBot = { id: "o", name: "Ops Bot" }
const ops = { id: "p", name: "Ops" }
const openbot = { id: "b", name: "openbot" }

describe("splitMentions", () => {
  it("highlights only the given targets, case-insensitively", () => {
    expect(splitMentions("hey @alpha and @Beta", [alpha])).toEqual([
      "hey ",
      { mention: alpha, text: "@alpha" },
      " and @Beta",
    ])
  })

  it("leaves email addresses and longer words alone", () => {
    expect(splitMentions("mail martin@openbot.dev", [openbot])).toEqual(["mail martin@openbot.dev"])
    expect(splitMentions("@openbotyard", [openbot])).toEqual(["@openbotyard"])
    expect(splitMentions("(@openbot), ok", [openbot])).toEqual([
      "(",
      { mention: openbot, text: "@openbot" },
      "), ok",
    ])
  })

  it("prefers the longest name and supports spaces", () => {
    expect(splitMentions("@Ops Bot please", [ops, opsBot])).toEqual([
      { mention: opsBot, text: "@Ops Bot" },
      " please",
    ])
  })

  it("resolves mention ids to names, skipping unknown agents", () => {
    const agents = new Map([["a", alpha]])
    expect(mentionTargets(["a", "gone"], agents)).toEqual([alpha])
  })
})

describe("autocomplete", () => {
  it("finds the query being typed after an @", () => {
    expect(activeMentionQuery("hi @Al", 6)).toEqual({ start: 3, query: "Al" })
    expect(activeMentionQuery("@", 1)).toEqual({ start: 0, query: "" })
    expect(activeMentionQuery("hi @Ops B", 9)).toEqual({ start: 3, query: "Ops B" })
    expect(activeMentionQuery("martin@ba", 9)).toBeNull()
    expect(activeMentionQuery("@ x", 3)).toBeNull()
    expect(activeMentionQuery("no at here", 10)).toBeNull()
  })

  it("filters by name prefix first, then by word prefix", () => {
    const all = [alpha, opsBot, openbot]
    expect(filterMentionCandidates(all, "")).toEqual(all)
    expect(filterMentionCandidates(all, "o")).toEqual([opsBot, openbot]) // name prefix
    expect(filterMentionCandidates(all, "b")).toEqual([opsBot]) // word prefix ("Ops Bot")
    expect(filterMentionCandidates(all, "AL")).toEqual([alpha])
    expect(filterMentionCandidates(all, "zz")).toEqual([])
  })

  it("inserts the exact name with a trailing space", () => {
    expect(insertMention("hi @al", 3, 6, "Alpha")).toEqual({ text: "hi @Alpha ", caret: 10 })
    expect(insertMention("@o rest", 0, 2, "Ops Bot")).toEqual({
      text: "@Ops Bot rest",
      caret: 9,
    })
  })
})

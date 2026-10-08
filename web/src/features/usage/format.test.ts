import { describe, expect, it } from "vitest"

import {
  cachedShare,
  formatCachedShare,
  formatPercent,
  formatTokens,
  parseDay,
  purposeLabel,
} from "./format"

describe("formatTokens", () => {
  it("keeps small numbers whole", () => {
    expect(formatTokens(0)).toBe("0")
    expect(formatTokens(7)).toBe("7")
    expect(formatTokens(999)).toBe("999")
  })

  it("shortens thousands and millions", () => {
    expect(formatTokens(1000)).toBe("1k")
    expect(formatTokens(1234)).toBe("1.2k")
    expect(formatTokens(23_400)).toBe("23.4k")
    expect(formatTokens(64_000)).toBe("64k")
    expect(formatTokens(234_567)).toBe("235k")
    expect(formatTokens(3_400_000)).toBe("3.4M")
    expect(formatTokens(1_000_000)).toBe("1M")
    expect(formatTokens(2_500_000_000)).toBe("2.5B")
  })

  it("moves to the next unit instead of printing 1000k", () => {
    expect(formatTokens(999_499)).toBe("999k")
    expect(formatTokens(999_600)).toBe("1M")
    expect(formatTokens(99_960)).toBe("100k")
  })
})

describe("percentages", () => {
  it("round to whole percents, flagging tiny shares", () => {
    expect(formatPercent(0)).toBe("0%")
    expect(formatPercent(0.452)).toBe("45%")
    expect(formatPercent(1)).toBe("100%")
    expect(formatPercent(0.001)).toBe("<1%")
    expect(formatPercent(0.999)).toBe(">99%")
  })

  it("cached share is cached over all input, or nothing without input", () => {
    expect(cachedShare({ promptTokens: 2000, cachedTokens: 500 })).toBe(0.25)
    expect(cachedShare({ promptTokens: 0, cachedTokens: 0 })).toBeNull()
    expect(formatCachedShare({ promptTokens: 2000, cachedTokens: 500 })).toBe("25%")
    expect(formatCachedShare({ promptTokens: 0, cachedTokens: 0 })).toBe("–")
  })
})

describe("labels and days", () => {
  it("names purposes in plain words", () => {
    expect(purposeLabel("turn")).toBe("Conversation")
    expect(purposeLabel("task")).toBe("Tasks & app events")
    expect(purposeLabel("compaction")).toBe("Summaries")
    expect(purposeLabel("subagent")).toBe("Subagents")
    expect(purposeLabel("embedding")).toBe("Embedding")
  })

  it("reads a day as a local date", () => {
    const day = parseDay("2026-10-08")
    expect([day.getFullYear(), day.getMonth(), day.getDate()]).toEqual([2026, 9, 8])
  })
})

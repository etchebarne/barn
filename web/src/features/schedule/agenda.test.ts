import { describe, expect, it } from "vitest"

import { buildAgenda, dayLabel, describeOutcome, formatDuration } from "./agenda"

const opts = { locale: "en-US", timeZone: "UTC" }
const now = new Date("2026-10-08T10:00:00Z")

describe("buildAgenda", () => {
  it("groups runs by day, earliest first", () => {
    const agenda = buildAgenda(
      [
        { taskId: "b", times: ["2026-10-09T09:00:00Z", "2026-10-08T15:00:00Z"], more: 0 },
        { taskId: "a", times: ["2026-10-08T12:00:00Z"], more: 0 },
      ],
      now,
      opts,
    )
    expect(agenda.frequent).toEqual([])
    expect(agenda.days.map((d) => d.label)).toEqual(["Today", "Tomorrow"])
    expect(agenda.days[0].entries.map((e) => e.taskId)).toEqual(["a", "b"])
  })

  it("pulls tasks that run many times a day aside", () => {
    const hourly = Array.from({ length: 24 }, (_, i) =>
      new Date(now.getTime() + i * 3600_000).toISOString(),
    )
    const agenda = buildAgenda(
      [
        { taskId: "hourly", times: hourly, more: 0 },
        { taskId: "minutely", times: [hourly[0]], more: 9000 },
      ],
      now,
      opts,
    )
    expect(agenda.days).toEqual([])
    expect(agenda.frequent.map((f) => [f.taskId, f.count, f.more])).toEqual([
      ["hourly", 24, false],
      ["minutely", 9001, true],
    ])
  })
})

describe("dayLabel", () => {
  it("names days after tomorrow by date", () => {
    expect(dayLabel(new Date("2026-10-11T08:00:00Z"), now, opts)).toBe("Sun, Oct 11")
  })
})

describe("formatDuration", () => {
  it.each([
    [900, "1s"],
    [65_000, "1m 5s"],
    [120_000, "2m"],
    [3_900_000, "1h 5m"],
  ])("%d ms → %s", (ms, want) => expect(formatDuration(ms)).toBe(want))
})

describe("describeOutcome", () => {
  it("marks failures", () => {
    expect(describeOutcome({ outcome: "failed" })).toEqual({ label: "Failed", tone: "bad" })
  })
})

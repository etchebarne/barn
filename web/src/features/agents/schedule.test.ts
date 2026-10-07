import { describe, expect, it } from "vitest"

import { describeSchedule, formatRelative, nextRunLabel, type Task } from "./schedule"

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "t1",
    agentId: "a1",
    name: "Weekday check-in",
    purpose: "Give Martin a quick catch-up",
    kind: "cron",
    cron: "1 10 * * 1-5",
    at: null,
    enabled: true,
    nextFireAt: null,
    lastFiredAt: null,
    ...overrides,
  }
}

const opts = { locale: "en-US", timeZone: "UTC" }

describe("describeSchedule", () => {
  it("describes cron schedules in plain English", () => {
    expect(describeSchedule(task(), opts)).toBe("At 10:01 AM, Monday through Friday")
  })

  it("falls back gracefully for an invalid cron", () => {
    expect(describeSchedule(task({ cron: "not cron" }), opts)).toBe("Cron: not cron")
  })

  it("describes one-off tasks with their date", () => {
    expect(
      describeSchedule(task({ kind: "once", cron: null, at: "2026-10-09T15:30:00Z" }), opts),
    ).toBe("Once on Fri, Oct 9, 3:30 PM")
  })

  it("describes signal tasks by their event, with no schedule", () => {
    expect(describeSchedule(task({ kind: "signal", cron: null, signal: "a PR is opened" }))).toBe(
      "When a PR is opened happens",
    )
    expect(describeSchedule(task({ kind: "signal", cron: null }))).toBe("When an event happens")
  })
})

describe("next run", () => {
  const now = new Date("2026-10-07T12:00:00Z")

  it("is relative, or Paused when disabled", () => {
    expect(nextRunLabel(task({ nextFireAt: "2026-10-07T15:00:00Z" }), now, "en-US")).toBe(
      "Next: in 3 hours",
    )
    expect(nextRunLabel(task({ enabled: false, nextFireAt: "2026-10-07T15:00:00Z" }), now)).toBe(
      "Paused",
    )
    expect(nextRunLabel(task({ nextFireAt: null }), now)).toBeNull()
  })

  it("calls a one-off task that already ran Done, not Paused", () => {
    const ran = task({
      kind: "once",
      cron: null,
      at: "2026-10-07T10:00:00Z",
      enabled: false,
      nextFireAt: null,
      lastFiredAt: "2026-10-07T10:00:01Z",
    })
    expect(nextRunLabel(ran, now)).toBe("Done")
  })

  it("picks a sensible unit", () => {
    expect(formatRelative("2026-10-08T12:00:00Z", now, "en-US")).toBe("tomorrow")
    expect(formatRelative("2026-10-07T12:05:00Z", now, "en-US")).toBe("in 5 minutes")
    expect(formatRelative("2026-10-07T12:00:20Z", now, "en-US")).toBe("this minute")
  })
})

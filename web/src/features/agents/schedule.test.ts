import { describe, expect, it } from "vitest"

import {
  cronFor,
  defaultDraft,
  describeDraft,
  describeSchedule,
  draftFromTask,
  formatRelative,
  nextRunLabel,
  scheduleBody,
  type Task,
} from "./schedule"

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
    signal: null,
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
    const signal = 'slack.app_mention in Slack (work) where channel contains "C0123"'
    expect(describeSchedule(task({ kind: "signal", cron: null, signal }))).toBe(`When ${signal}`)
    expect(describeSchedule(task({ kind: "signal", cron: null, signal: null }))).toBe(
      "When a connected app sends an event",
    )
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

describe("schedule drafts", () => {
  const now = new Date(2026, 9, 7, 12, 0) // Wednesday, local time
  const base = defaultDraft(now)
  const of = (cron: string) => draftFromTask(task({ cron }), now)

  it("starts new tasks every day at 9, with Once on tomorrow", () => {
    expect(cronFor(base)).toBe("0 9 * * *")
    expect(base).toMatchObject({ weekday: 3, date: "2026-10-08", onceTime: "09:00" })
  })

  it("builds each preset's cron", () => {
    const at = { ...base, time: "07:05" }
    expect(cronFor({ ...at, preset: "weekdays" })).toBe("5 7 * * 1-5")
    expect(cronFor({ ...at, preset: "weekly", weekday: 1 })).toBe("5 7 * * 1")
    expect(cronFor({ ...at, preset: "hourly" })).toBe("5 * * * *")
    expect(cronFor({ ...at, preset: "custom", cron: " 0  */2 * * * " })).toBe("0 */2 * * *")
  })

  it("sends exactly one of cron or at", () => {
    expect(scheduleBody(base)).toEqual({ cron: "0 9 * * *" })
    expect(scheduleBody({ ...base, mode: "once", date: "2026-10-20", onceTime: "15:30" })).toEqual({
      at: "2026-10-20 15:30",
    })
  })

  it("recognizes preset crons when editing, and falls back to Custom", () => {
    expect(of("30 8 * * 1-5")).toMatchObject({ preset: "weekdays", time: "08:30" })
    expect(of("0 18 * * 0")).toMatchObject({ preset: "weekly", weekday: 0, time: "18:00" })
    expect(of("45 * * * *")).toMatchObject({ preset: "hourly", time: "00:45" })
    expect(of("0 9 1 * *")).toMatchObject({ preset: "custom", cron: "0 9 1 * *" })
  })

  it("describes drafts, or nothing for an invalid custom cron", () => {
    expect(describeDraft({ ...base, preset: "weekdays", time: "08:30" })).toBe(
      "At 08:30 AM, Monday through Friday",
    )
    expect(describeDraft({ ...base, preset: "custom", cron: "nope" })).toBeNull()
  })
})

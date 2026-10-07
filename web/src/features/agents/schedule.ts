import cronstrue from "cronstrue"

import type { Schemas } from "@/lib/api-client"

export type Task = Schemas["Task"]

type Options = { locale?: string; timeZone?: string }

/** "Thu, Oct 9, 3:30 PM" */
export function formatDateTime(iso: string, { locale, timeZone }: Options = {}): string {
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    timeZone,
  }).format(new Date(iso))
}

/** A cron expression in words ("At 09:00 AM, Monday through Friday"), or null if invalid. */
export function describeCron(cron: string): string | null {
  if (cron.trim().split(/\s+/).length !== 5) return null
  try {
    return cronstrue.toString(cron, { use24HourTimeFormat: false, verbose: false })
  } catch {
    return null
  }
}

/**
 * The schedule in plain English: "At 10:01 AM, Monday through Friday", "Once on Thu, Oct 9,
 * 3:30 PM", or "When slack.app_mention in Slack (work)" for signal tasks.
 */
export function describeSchedule(task: Task, options: Options = {}): string {
  switch (task.kind) {
    case "cron":
      if (!task.cron) return "On a schedule"
      return describeCron(task.cron) ?? `Cron: ${task.cron}`
    case "once":
      return task.at ? `Once on ${formatDateTime(task.at, options)}` : "Once"
    case "signal":
      return task.signal?.trim()
        ? `When ${task.signal.trim()}`
        : "When a connected app sends an event"
    default:
      return "Scheduled"
  }
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
]

/** "in 3 hours", "tomorrow", "in 5 minutes", "now". */
export function formatRelative(iso: string, now: Date = new Date(), locale?: string): string {
  const seconds = (new Date(iso).getTime() - now.getTime()) / 1000
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" })
  for (const [unit, size] of UNITS) {
    if (Math.abs(seconds) >= size) return format.format(Math.round(seconds / size), unit)
  }
  return format.format(0, "minute")
}

/** "Next: in 3 hours", "Paused", "Done" (a one-off that already ran), or nothing (signal tasks). */
export function nextRunLabel(task: Task, now: Date = new Date(), locale?: string): string | null {
  if (task.kind === "once" && task.lastFiredAt) return "Done"
  if (!task.enabled) return "Paused"
  if (!task.nextFireAt) return null
  return `Next: ${formatRelative(task.nextFireAt, now, locale)}`
}

// Editing a schedule -------------------------------------------------------------------------

export type RepeatPreset = "daily" | "weekdays" | "weekly" | "hourly" | "custom"

/** What the task form edits; turned into `cron` or `at` for the API. */
export type ScheduleDraft = {
  mode: "repeat" | "once"
  preset: RepeatPreset
  /** "HH:MM" for daily, weekdays and weekly; hourly uses only its minutes. */
  time: string
  /** 0 = Sunday … 6 = Saturday (weekly). */
  weekday: number
  /** The expression for "Custom cron". */
  cron: string
  /** "YYYY-MM-DD" and "HH:MM", local (once). */
  date: string
  onceTime: string
}

const pad = (n: number) => String(n).padStart(2, "0")

/** "2026-10-09" in local time. */
export function localDate(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function localTime(date: Date): string {
  return `${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/** A new task: every day at 9:00; "Once" starts on tomorrow at 9:00. */
export function defaultDraft(now: Date = new Date()): ScheduleDraft {
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  return {
    mode: "repeat",
    preset: "daily",
    time: "09:00",
    weekday: now.getDay(),
    cron: "0 9 * * *",
    date: localDate(tomorrow),
    onceTime: "09:00",
  }
}

function hoursAndMinutes(time: string): [number, number] {
  const [h = "0", m = "0"] = time.split(":")
  return [Number(h) || 0, Number(m) || 0]
}

/** The 5-field cron (minute hour day month weekday) for a repeating draft. */
export function cronFor(draft: ScheduleDraft): string {
  const [hour, minute] = hoursAndMinutes(draft.time)
  switch (draft.preset) {
    case "daily":
      return `${minute} ${hour} * * *`
    case "weekdays":
      return `${minute} ${hour} * * 1-5`
    case "weekly":
      return `${minute} ${hour} * * ${draft.weekday}`
    case "hourly":
      return `${minute} * * * *`
    default:
      return draft.cron.trim().replace(/\s+/g, " ")
  }
}

/** The schedule part of a create/update request: exactly one of `cron` or `at`. */
export function scheduleBody(draft: ScheduleDraft): { cron: string } | { at: string } {
  return draft.mode === "repeat"
    ? { cron: cronFor(draft) }
    : { at: `${draft.date} ${draft.onceTime}` }
}

/** The draft in words, for the form's preview. Null when it can't be described yet. */
export function describeDraft(draft: ScheduleDraft, options: Options = {}): string | null {
  if (draft.mode === "repeat") return describeCron(cronFor(draft))
  if (!draft.date || !draft.onceTime) return null
  const at = new Date(`${draft.date}T${draft.onceTime}`)
  return Number.isNaN(at.getTime()) ? null : `Once on ${formatDateTime(at.toISOString(), options)}`
}

const NUMBER = /^\d+$/

/** Recognizes the presets' own expressions; anything else is "Custom cron". */
function parseCron(cron: string): Partial<ScheduleDraft> {
  const fields = cron.trim().split(/\s+/)
  if (fields.length === 5) {
    const [minute = "", hour = "", day = "", month = "", weekday = ""] = fields
    if (NUMBER.test(minute) && day === "*" && month === "*") {
      if (hour === "*" && weekday === "*") {
        return { preset: "hourly", time: `00:${pad(Number(minute))}`, cron }
      }
      if (NUMBER.test(hour)) {
        const time = `${pad(Number(hour))}:${pad(Number(minute))}`
        if (weekday === "*") return { preset: "daily", time, cron }
        if (weekday === "1-5") return { preset: "weekdays", time, cron }
        if (/^[0-7]$/.test(weekday)) {
          return { preset: "weekly", time, weekday: Number(weekday) % 7, cron }
        }
      }
    }
  }
  return { preset: "custom", cron }
}

/** The form's starting point for an existing task. */
export function draftFromTask(task: Task, now: Date = new Date()): ScheduleDraft {
  const base = defaultDraft(now)
  if (task.kind === "once" && task.at) {
    const at = new Date(task.at)
    return { ...base, mode: "once", date: localDate(at), onceTime: localTime(at) }
  }
  if (task.kind === "cron" && task.cron) return { ...base, ...parseCron(task.cron) }
  return base
}

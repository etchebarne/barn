import cronstrue from "cronstrue"

import type { Schemas } from "@/lib/api-client"

/** A task as listed by the API. `signal` tasks arrive with connectors; handle them already. */
export type Task = Omit<Schemas["Task"], "kind"> & {
  kind: Schemas["Task"]["kind"] | "signal"
  /** For signal tasks: what triggers it, in words (e.g. "a PR is opened"). */
  signal?: string | null
}

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

/**
 * The schedule in plain English: "At 10:01 AM, Monday through Friday", "Once on Thu, Oct 9,
 * 3:30 PM", or "When a PR is opened happens"-style text for signal tasks.
 */
export function describeSchedule(task: Task, options: Options = {}): string {
  switch (task.kind) {
    case "cron":
      if (!task.cron) return "On a schedule"
      try {
        return cronstrue.toString(task.cron, { use24HourTimeFormat: false, verbose: false })
      } catch {
        return `Cron: ${task.cron}`
      }
    case "once":
      return task.at ? `Once on ${formatDateTime(task.at, options)}` : "Once"
    case "signal":
      return `When ${task.signal?.trim() || "an event"} happens`
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

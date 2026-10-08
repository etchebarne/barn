import type { TaskRun, UpcomingRuns } from "./api"

/** More runs a day than this and a task is "frequent": listed once, not at every time. */
export const FREQUENT_PER_DAY = 6

export type AgendaEntry = { taskId: string; time: Date }
export type AgendaDay = { key: string; label: string; entries: AgendaEntry[] }
export type Agenda = {
  /** Tasks that run often (every few minutes, hourly…), with their next run. */
  frequent: { taskId: string; next: Date; count: number; more: boolean }[]
  /** The other runs, by day, earliest first. */
  days: AgendaDay[]
}

type Options = { locale?: string; timeZone?: string }

/** "2026-10-08" in the given time zone: the day a moment falls on. */
function dayKey(date: Date, timeZone?: string): string {
  // en-CA formats dates as YYYY-MM-DD.
  return new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date)
}

/** "Today", "Tomorrow", or "Wed, Oct 15". */
export function dayLabel(date: Date, now: Date, { locale, timeZone }: Options = {}): string {
  const key = dayKey(date, timeZone)
  if (key === dayKey(now, timeZone)) return "Today"
  if (key === dayKey(new Date(now.getTime() + 24 * 3600 * 1000), timeZone)) return "Tomorrow"
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
    timeZone,
  }).format(date)
}

/** Lays the coming runs out by day, pulling tasks that run many times a day aside. */
export function buildAgenda(upcoming: UpcomingRuns[], now: Date, options: Options = {}): Agenda {
  const frequent: Agenda["frequent"] = []
  const byDay = new Map<string, AgendaDay>()
  for (const u of upcoming) {
    const times = u.times.map((t) => new Date(t))
    if (times.length === 0) continue
    const perDay = new Map<string, number>()
    for (const t of times) {
      const key = dayKey(t, options.timeZone)
      perDay.set(key, (perDay.get(key) ?? 0) + 1)
    }
    if (u.more > 0 || Math.max(...perDay.values()) > FREQUENT_PER_DAY) {
      frequent.push({
        taskId: u.taskId,
        next: times[0],
        count: times.length + u.more,
        more: u.more > 0,
      })
      continue
    }
    for (const t of times) {
      const key = dayKey(t, options.timeZone)
      let day = byDay.get(key)
      if (!day) {
        day = { key, label: dayLabel(t, now, options), entries: [] }
        byDay.set(key, day)
      }
      day.entries.push({ taskId: u.taskId, time: t })
    }
  }
  const days = [...byDay.values()].toSorted((a, b) => a.key.localeCompare(b.key))
  for (const day of days) day.entries.sort((a, b) => a.time.getTime() - b.time.getTime())
  frequent.sort((a, b) => a.next.getTime() - b.next.getTime())
  return { frequent, days }
}

/** "3:30 PM" */
export function formatTime(date: Date, { locale, timeZone }: Options = {}): string {
  return new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit", timeZone }).format(
    date,
  )
}

/** "12s", "3m 4s", "1h 5m": how long a run took. */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`
  return m % 60 ? `${Math.floor(m / 60)}h ${m % 60}m` : `${Math.floor(m / 60)}h`
}

export type OutcomeTone = "live" | "muted" | "good" | "bad"

const OUTCOMES: Record<TaskRun["outcome"], { label: string; tone: OutcomeTone }> = {
  running: { label: "Running", tone: "live" },
  acted: { label: "Did something", tone: "good" },
  quiet: { label: "Nothing to do", tone: "muted" },
  unchanged: { label: "No change", tone: "muted" },
  stopped: { label: "Stopped", tone: "muted" },
  failed: { label: "Failed", tone: "bad" },
}

/** How a run ended, in words, and the tone to show it in. */
export function describeOutcome(run: Pick<TaskRun, "outcome">): {
  label: string
  tone: OutcomeTone
} {
  return OUTCOMES[run.outcome]
}

/** "by you" (Run now), "on an event", or nothing for a scheduled run. */
export function describeTrigger(run: Pick<TaskRun, "trigger">): string | null {
  if (run.trigger === "manual") return "Run by you"
  if (run.trigger === "signal") return "On an app event"
  return null
}

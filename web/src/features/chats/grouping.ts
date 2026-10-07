import type { Message } from "@/lib/api-client"

const RUN_GAP_MS = 5 * 60_000

export type Row = {
  message: Message
  /** First message of a run of consecutive messages from the same author. */
  startsRun: boolean
  /** Last message of a run (where the agent avatar sits). */
  endsRun: boolean
  /** First message of a calendar day: render a date separator above it. */
  startsDay: boolean
}

function sameAuthor(a: Message, b: Message) {
  return a.author.kind === b.author.kind && a.author.agentId === b.author.agentId
}

function dayKey(iso: string) {
  const d = new Date(iso)
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}

function continues(prev: Message | undefined, next: Message | undefined) {
  if (!prev || !next) return false
  if (prev.author.kind === "system" || next.author.kind === "system") return false
  return (
    sameAuthor(prev, next) &&
    dayKey(prev.createdAt) === dayKey(next.createdAt) &&
    Date.parse(next.createdAt) - Date.parse(prev.createdAt) < RUN_GAP_MS
  )
}

/** Annotates chronological messages with run and day boundaries for layout. */
export function toRows(messages: Message[]): Row[] {
  return messages.map((message, i) => {
    const prev = messages[i - 1]
    const next = messages[i + 1]
    return {
      message,
      startsRun: !continues(prev, message),
      endsRun: !continues(message, next),
      startsDay: !prev || dayKey(prev.createdAt) !== dayKey(message.createdAt),
    }
  })
}

const timeFormat = new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" })
const dayFormat = new Intl.DateTimeFormat(undefined, {
  weekday: "long",
  month: "short",
  day: "numeric",
})
const dayWithYearFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" })

export function formatTime(iso: string) {
  return timeFormat.format(new Date(iso))
}

export function formatDay(iso: string, now = new Date()) {
  const date = new Date(iso)
  const today = dayKey(now.toISOString())
  const yesterday = new Date(now)
  yesterday.setDate(now.getDate() - 1)
  const key = dayKey(iso)
  if (key === today) return "Today"
  if (key === dayKey(yesterday.toISOString())) return "Yesterday"
  return date.getFullYear() === now.getFullYear()
    ? dayFormat.format(date)
    : dayWithYearFormat.format(date)
}

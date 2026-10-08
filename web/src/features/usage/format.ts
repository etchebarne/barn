import type { UsageTotals } from "./api"

function compact(value: number, suffix: string): string {
  // One decimal below 100 (23.4k), whole numbers above (234k); never "64.0k".
  const rounded = value < 99.95 ? Math.round(value * 10) / 10 : Math.round(value)
  return `${rounded}${suffix}`
}

/** Token and call counts, short: 950, 1.2k, 23.4k, 234k, 3.4M, 1.2B. */
export function formatTokens(n: number): string {
  const abs = Math.abs(n)
  if (abs < 1000) return String(Math.round(n))
  // Thresholds sit where rounding would otherwise print "1000k".
  if (abs < 999_500) return compact(n / 1e3, "k")
  if (abs < 999_500_000) return compact(n / 1e6, "M")
  return compact(n / 1e9, "B")
}

/** A 0–1 ratio as a whole percentage; tiny non-zero shares read "<1%" rather than "0%". */
export function formatPercent(ratio: number): string {
  if (ratio > 0 && ratio < 0.005) return "<1%"
  if (ratio < 1 && ratio > 0.995) return ">99%"
  return `${Math.round(ratio * 100)}%`
}

/** Share of input read from the provider's cache, or null when there was no input. */
export function cachedShare(totals: Pick<UsageTotals, "promptTokens" | "cachedTokens">) {
  return totals.promptTokens > 0 ? totals.cachedTokens / totals.promptTokens : null
}

/** "45%", or "–" when there was no input to cache. */
export function formatCachedShare(totals: Pick<UsageTotals, "promptTokens" | "cachedTokens">) {
  const share = cachedShare(totals)
  return share === null ? "–" : formatPercent(share)
}

/** Why each kind of model call happened, in the order they're listed. */
export const PURPOSES = [
  { purpose: "turn", label: "Conversation" },
  { purpose: "task", label: "Tasks & app events" },
  { purpose: "compaction", label: "Summaries" },
  { purpose: "subagent", label: "Subagents" },
] as const

export function purposeLabel(purpose: string): string {
  const known = PURPOSES.find((p) => p.purpose === purpose)
  if (known) return known.label
  return purpose ? purpose.charAt(0).toUpperCase() + purpose.slice(1) : "Other"
}

/** A `YYYY-MM-DD` day as a local date (not UTC midnight, which can land on the previous day). */
export function parseDay(day: string): Date {
  const [y = 0, m = 1, d = 1] = day.split("-").map(Number)
  return new Date(y, m - 1, d)
}

export const CACHE_EXPLAINER =
  "Input the provider already had cached from earlier requests. Cached input is cheaper and counts less toward your plan's limits."

import { cn } from "cn"

import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

import type { DayUsage, PurposeUsage, UsageReport, UsageTotals } from "./api"
import {
  CACHE_EXPLAINER,
  PURPOSES,
  formatCachedShare,
  formatPercent,
  formatTokens,
  parseDay,
  purposeLabel,
} from "./format"

/** "From cache", with a tooltip explaining why it matters. */
export function CachedLabel({ children = "From cache" }: { children?: string }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <button
            type="button"
            className="cursor-help text-left underline decoration-muted-foreground/50 decoration-dotted underline-offset-4"
          >
            {children}
          </button>
        }
      />
      <TooltipContent className="block">{CACHE_EXPLAINER}</TooltipContent>
    </Tooltip>
  )
}

/** How full the agent's context is, measured against the point where it gets summarized. */
export function ContextMeter({ context }: { context: NonNullable<UsageReport["context"]> }) {
  const { tokens, window, compactAt } = context
  const ratio = compactAt > 0 ? Math.min(tokens / compactAt, 1) : 0
  return (
    <div className="flex flex-col gap-2 rounded-lg bg-muted/50 px-3 py-2.5 text-sm">
      {tokens > 0 ? (
        <>
          <p className="tabular-nums">
            Context: {formatTokens(tokens)}
            <span className="text-muted-foreground">
              {" "}
              of {formatTokens(compactAt)} before summarizing
            </span>
          </p>
          <div
            role="meter"
            aria-label="Context used before summarizing"
            aria-valuemin={0}
            aria-valuemax={compactAt}
            aria-valuenow={Math.min(tokens, compactAt)}
            aria-valuetext={`${formatTokens(tokens)} of ${formatTokens(compactAt)}`}
            className="h-1.5 overflow-hidden rounded-full bg-muted"
          >
            <div className="h-full rounded-full bg-primary" style={{ width: `${ratio * 100}%` }} />
          </div>
        </>
      ) : (
        <p>
          Context: <span className="text-muted-foreground">No requests yet</span>
        </p>
      )}
      <p className="text-xs text-muted-foreground tabular-nums">
        Model window: {formatTokens(window)}
      </p>
    </div>
  )
}

const longDay = new Intl.DateTimeFormat(undefined, {
  weekday: "short",
  month: "short",
  day: "numeric",
})
const shortDay = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" })
const weekday = new Intl.DateTimeFormat(undefined, { weekday: "short" })

function dayNumbers(day: UsageTotals): string {
  return `${formatTokens(day.promptTokens)} input, ${formatCachedShare(day)} from cache, ${formatTokens(day.completionTokens)} output, ${formatTokens(day.calls)} ${day.calls === 1 ? "call" : "calls"}`
}

/**
 * Input tokens per day as bars, with the part read from cache shaded lighter. Hovering or
 * focusing a bar shows that day's numbers. Days are oldest first; the last one is today.
 */
export function DailyBars({ days, className }: { days: DayUsage[]; className?: string }) {
  const max = Math.max(0, ...days.map((d) => d.promptTokens))
  const labelEach = days.length <= 7
  const last = days.length - 1

  return (
    <figure className={cn("flex flex-col gap-1.5", className)}>
      {/* Grouped tooltips: no delay while moving across the bars. */}
      <TooltipProvider delay={0} closeDelay={0}>
        <div className={cn("flex h-24 items-stretch", labelEach ? "gap-1.5" : "gap-0.5")}>
          {days.map((day, i) => {
            const date = parseDay(day.date)
            const height = max > 0 ? (day.promptTokens / max) * 100 : 0
            const cached = day.promptTokens > 0 ? (day.cachedTokens / day.promptTokens) * 100 : 0
            const name = i === last ? "Today" : longDay.format(date)
            return (
              <Tooltip key={day.date}>
                <TooltipTrigger
                  render={
                    <button
                      type="button"
                      aria-label={`${name}: ${dayNumbers(day)}`}
                      className="group/bar flex min-w-0 flex-1 cursor-default flex-col justify-end rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                    />
                  }
                >
                  {day.promptTokens > 0 ? (
                    <span
                      className="flex min-h-0.5 flex-col gap-px overflow-hidden rounded-[3px]"
                      style={{ height: `${height}%` }}
                    >
                      <span className="min-h-px flex-1 bg-primary/80 group-hover/bar:bg-primary group-focus-visible/bar:bg-primary" />
                      {cached > 0 && (
                        <span
                          className="shrink-0 bg-primary/30 group-hover/bar:bg-primary/40 group-focus-visible/bar:bg-primary/40"
                          style={{ height: `${cached}%` }}
                        />
                      )}
                    </span>
                  ) : (
                    <span className="h-px bg-border" />
                  )}
                </TooltipTrigger>
                <TooltipContent className="flex-col items-start gap-0.5">
                  <span className="font-medium">{name}</span>
                  <span className="tabular-nums">
                    {formatTokens(day.promptTokens)} input · {formatCachedShare(day)} from cache
                  </span>
                  <span className="tabular-nums">
                    {formatTokens(day.completionTokens)} output · {formatTokens(day.calls)}{" "}
                    {day.calls === 1 ? "call" : "calls"}
                  </span>
                </TooltipContent>
              </Tooltip>
            )
          })}
        </div>
      </TooltipProvider>
      {labelEach ? (
        <div className="flex gap-1.5 text-xs text-muted-foreground" aria-hidden="true">
          {days.map((day, i) => (
            <span key={day.date} className="min-w-0 flex-1 truncate text-center">
              {i === last ? "Today" : weekday.format(parseDay(day.date))}
            </span>
          ))}
        </div>
      ) : (
        days.length > 0 && (
          <div className="flex justify-between text-xs text-muted-foreground" aria-hidden="true">
            <span>{shortDay.format(parseDay(days[0].date))}</span>
            <span>Today</span>
          </div>
        )
      )}
      <figcaption className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <span className="size-2 rounded-[2px] bg-primary/80" aria-hidden="true" />
          Input tokens per day
        </span>
        <span className="flex items-center gap-1.5">
          <span className="size-2 rounded-[2px] bg-primary/30" aria-hidden="true" />
          From cache
        </span>
      </figcaption>
    </figure>
  )
}

function purposeOrder(purpose: string): number {
  const i = PURPOSES.findIndex((p) => p.purpose === purpose)
  return i === -1 ? PURPOSES.length : i
}

/** Where input tokens went, by why the model was called. Purposes with no calls are left out. */
export function PurposeBreakdown({ purposes }: { purposes: PurposeUsage[] }) {
  const total = purposes.reduce((sum, p) => sum + p.promptTokens, 0)
  const rows = purposes
    .filter((p) => p.calls > 0)
    .toSorted((a, b) => purposeOrder(a.purpose) - purposeOrder(b.purpose))
  if (rows.length === 0) return null

  return (
    <ul className="flex flex-col gap-2 text-sm" aria-label="Input tokens by purpose">
      {rows.map((row) => {
        const share = total > 0 ? row.promptTokens / total : 0
        return (
          <li
            key={row.purpose}
            className="grid grid-cols-[minmax(0,9rem)_1fr_5.5rem] items-center gap-3"
          >
            <span className="truncate">{purposeLabel(row.purpose)}</span>
            <span className="h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden="true">
              <span
                className="block h-full rounded-full bg-primary/80"
                style={{ width: `${share * 100}%` }}
              />
            </span>
            <span className="text-right text-muted-foreground tabular-nums">
              {formatTokens(row.promptTokens)}{" "}
              <span className="inline-block w-10">{formatPercent(share)}</span>
            </span>
          </li>
        )
      })}
    </ul>
  )
}

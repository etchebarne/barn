import { Skeleton } from "@/components/ui/skeleton"
import type { Agent } from "@/lib/api-client"

import { useAgentUsage, type UsageTotals } from "./api"
import { formatCachedShare, formatTokens } from "./format"
import { CachedLabel, ContextMeter, DailyBars, PurposeBreakdown } from "./usage-charts"

const DAYS = 7

const ROWS: { label: string; cached?: true; value: (t: UsageTotals) => string }[] = [
  { label: "Input tokens", value: (t) => formatTokens(t.promptTokens) },
  { label: "From cache", cached: true, value: formatCachedShare },
  { label: "Output tokens", value: (t) => formatTokens(t.completionTokens) },
  { label: "Model calls", value: (t) => formatTokens(t.calls) },
]

/** Today's and this week's numbers side by side. */
function TotalsTable({ today, week }: { today: UsageTotals; week: UsageTotals }) {
  return (
    <table className="w-full text-sm tabular-nums">
      <thead>
        <tr className="text-xs text-muted-foreground">
          <th scope="col" className="pb-1 text-left font-normal">
            <span className="sr-only">Measure</span>
          </th>
          <th scope="col" className="w-24 pb-1 text-right font-normal">
            Today
          </th>
          <th scope="col" className="w-24 pb-1 text-right font-normal">
            Last {DAYS} days
          </th>
        </tr>
      </thead>
      <tbody>
        {ROWS.map((row) => (
          <tr key={row.label}>
            <th scope="row" className="py-0.5 text-left font-normal text-muted-foreground">
              {row.cached ? <CachedLabel /> : row.label}
            </th>
            <td className="py-0.5 text-right">{row.value(today)}</td>
            <td className="py-0.5 text-right">{row.value(week)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

/** Tokens the agent used: how full its context is now, and its last week by day and purpose. */
export function AgentUsageSection({ agent }: { agent: Agent }) {
  const { data: usage, isPending, error } = useAgentUsage(agent.id, DAYS)

  return (
    <section className="flex flex-col gap-3" aria-labelledby="agent-usage">
      <h3 id="agent-usage" className="text-sm font-medium">
        Usage
      </h3>
      {isPending ? (
        <Skeleton className="h-40 w-full rounded-lg" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load usage: {error.message}</p>
      ) : (
        <>
          {usage.context && <ContextMeter context={usage.context} />}
          {usage.total.calls === 0 ? (
            <p className="text-sm text-muted-foreground">No model calls in the last {DAYS} days.</p>
          ) : (
            <>
              <TotalsTable today={usage.today} week={usage.total} />
              <DailyBars days={usage.days} />
              <PurposeBreakdown purposes={usage.byPurpose} />
            </>
          )}
        </>
      )}
    </section>
  )
}

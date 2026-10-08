import { cn } from "cn"
import { ChartColumnIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { Skeleton } from "@/components/ui/skeleton"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { AgentAvatar, openAgentDetails, useAgentsById } from "@/features/agents"
import {
  CachedLabel,
  DailyBars,
  PurposeBreakdown,
  formatCachedShare,
  formatTokens,
  useUsage,
  type AgentUsage,
  type UsageTotals,
} from "@/features/usage"

import { SettingsSection } from "./settings-section"

const PERIODS = [7, 30] as const
type Period = (typeof PERIODS)[number]

function toPeriod(value: unknown): Period | null {
  return PERIODS.find((p) => String(p) === value) ?? null
}

function Stat({ label, value }: { label: ReactNode; value: string }) {
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border px-3 py-2.5">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="text-lg font-medium tabular-nums">{value}</span>
    </div>
  )
}

function Totals({ totals }: { totals: UsageTotals }) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <Stat label="Input tokens" value={formatTokens(totals.promptTokens)} />
      <Stat label={<CachedLabel />} value={formatCachedShare(totals)} />
      <Stat label="Output tokens" value={formatTokens(totals.completionTokens)} />
      <Stat label="Model calls" value={formatTokens(totals.calls)} />
    </div>
  )
}

/** Every agent's share, most input first. A row opens that agent's details. */
function AgentTable({ rows }: { rows: AgentUsage[] }) {
  const agents = useAgentsById()
  const sorted = rows.filter((r) => r.calls > 0).toSorted((a, b) => b.promptTokens - a.promptTokens)
  const numeric = "py-1.5 pl-2 text-right tabular-nums"

  return (
    <table className="w-full table-fixed text-sm" aria-label="Usage by agent">
      <thead>
        <tr className="text-xs text-muted-foreground">
          <th scope="col" className="pb-1 text-left font-normal">
            Agent
          </th>
          <th scope="col" className="w-16 pb-1 pl-2 text-right font-normal sm:w-20">
            Input
          </th>
          <th scope="col" className="w-16 pb-1 pl-2 text-right font-normal sm:w-20">
            Cache
          </th>
          <th scope="col" className="hidden w-20 pb-1 pl-2 text-right font-normal sm:table-cell">
            Output
          </th>
          <th scope="col" className="w-12 pb-1 pl-2 text-right font-normal sm:w-16">
            Calls
          </th>
        </tr>
      </thead>
      <tbody>
        {sorted.map((row) => {
          const agent = agents.get(row.agentId)
          return (
            <tr
              key={row.agentId}
              className="relative hover:bg-muted/50 has-focus-visible:bg-muted/50"
            >
              <td className="py-1.5 pr-2">
                {agent ? (
                  // Stretched over the whole row, so any cell opens the agent.
                  <button
                    type="button"
                    className="flex w-full min-w-0 items-center gap-2 text-left outline-none select-none before:absolute before:inset-0 before:rounded-md focus-visible:before:ring-2 focus-visible:before:ring-ring/50"
                    onClick={() => openAgentDetails(agent.id)}
                  >
                    <AgentAvatar id={agent.id} name={agent.name} size="sm" />
                    <span className="truncate">{agent.name}</span>
                  </button>
                ) : (
                  <span className="flex min-w-0 items-center gap-2 text-muted-foreground">
                    <AgentAvatar name="?" size="sm" />
                    <span className="truncate">Deleted agent</span>
                  </span>
                )}
              </td>
              <td className={numeric}>{formatTokens(row.promptTokens)}</td>
              <td className={numeric}>{formatCachedShare(row)}</td>
              <td className={cn(numeric, "hidden sm:table-cell")}>
                {formatTokens(row.completionTokens)}
              </td>
              <td className={numeric}>{formatTokens(row.calls)}</td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

function Subheading({ children }: { children: ReactNode }) {
  return <h3 className="text-xs font-medium text-muted-foreground">{children}</h3>
}

/** Tokens used across all agents over the last 7 or 30 days. No cost: the plan has limits, not prices. */
export function UsageSection() {
  const [days, setDays] = useState<Period>(7)
  const { data: usage, isPending, error } = useUsage(days)

  return (
    <SettingsSection
      id="usage"
      title="Usage"
      description="Tokens your agents used. Cached input is cheaper and counts less toward your plan's limits."
    >
      <ToggleGroup
        variant="outline"
        size="sm"
        aria-label="Period"
        value={[String(days)]}
        onValueChange={(value: unknown[]) => {
          // Ignore deselecting the active option: one period is always selected.
          const next = toPeriod(value[0])
          if (next) setDays(next)
        }}
      >
        {PERIODS.map((period) => (
          <ToggleGroupItem key={period} value={String(period)} className="select-none">
            {period} days
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {isPending ? (
        <Skeleton className="h-48 w-full" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load usage: {error.message}</p>
      ) : usage.total.calls === 0 ? (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <ChartColumnIcon className="size-4 shrink-0" aria-hidden="true" />
          No model calls in the last {days} days. Usage shows up here once your agents get to work.
        </p>
      ) : (
        <>
          <Totals totals={usage.total} />
          <DailyBars days={usage.days} />
          <div className="flex flex-col gap-2">
            <Subheading>By agent</Subheading>
            <AgentTable rows={usage.byAgent} />
          </div>
          <div className="flex flex-col gap-2">
            <Subheading>By purpose</Subheading>
            <PurposeBreakdown purposes={usage.byPurpose} />
          </div>
        </>
      )}
    </SettingsSection>
  )
}

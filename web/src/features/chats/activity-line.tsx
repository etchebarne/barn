import { useEffect, useState } from "react"

import { activityLabel, AgentAvatar } from "@/features/agents"
import type { Agent } from "@/lib/api-client"

/** Wait this long before showing work, so quick turns don't flash a line. */
const SHOW_DELAY_MS = 120
/** Keep the line this long after work ends, so a turn that resumes doesn't flicker. */
const HIDE_DELAY_MS = 500
/** Only count the time once the work is long enough for the count to be news. */
const ELAPSED_AFTER_S = 5

/** Agents shown as working, with the show and hide delays applied. */
function useShownWorkers(agents: Agent[]): Agent[] {
  const working = agents.filter((a) => a.activity.state === "working")
  const workingKey = working.map((a) => a.id).join(",")
  const [shown, setShown] = useState<string[]>([])

  useEffect(() => {
    const ids = workingKey === "" ? [] : workingKey.split(",")
    const adding = ids.some((id) => !shown.includes(id))
    const removing = shown.some((id) => !ids.includes(id))
    if (!adding && !removing) return undefined
    const timer = setTimeout(
      () =>
        setShown((prev) => [
          ...prev.filter((id) => ids.includes(id)),
          ...ids.filter((id) => !prev.includes(id)),
        ]),
      adding ? SHOW_DELAY_MS : HIDE_DELAY_MS,
    )
    return () => clearTimeout(timer)
  }, [workingKey, shown])

  // Lingering agents keep their last known state; they're idle now but still listed.
  return shown.flatMap((id) => agents.find((a) => a.id === id) ?? [])
}

/** "12s", "1m 05s": how long the agent has been at it. */
export function formatElapsed(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}m ${String(s).padStart(2, "0")}s`
}

function useElapsedSeconds(since: string | null | undefined): number | null {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!since) return undefined
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [since])
  if (!since) return null
  return Math.max(0, Math.floor((now - Date.parse(since)) / 1000))
}

function WorkingRow({ agent, named }: { agent: Agent; named: boolean }) {
  // While lingering after the work ended, keep saying what it was doing last.
  const current = activityLabel(agent)
  const [lastLabel, setLastLabel] = useState(current ?? "thinking…")
  if (current && current !== lastLabel) setLastLabel(current)
  const label = current ?? lastLabel
  const elapsed = useElapsedSeconds(agent.activity.since)
  return (
    <div className="flex h-8 animate-in items-center gap-2.5 duration-(--duration-popover) ease-out-quint fade-in-0 slide-in-from-bottom-1">
      <AgentAvatar id={agent.id} name={agent.name} size="sm" active />
      <p className="min-w-0 truncate text-[13px] text-muted-foreground">
        {named && <span className="font-medium text-foreground/85">{agent.name} </span>}
        <span className="shimmer">{named ? `is ${label}` : capitalize(label)}</span>
      </p>
      {elapsed !== null && elapsed >= ELAPSED_AFTER_S && (
        <span className="shrink-0 text-xs text-muted-foreground/70 tabular-nums">
          {formatElapsed(elapsed)}
        </span>
      )}
    </div>
  )
}

function capitalize(text: string) {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

/**
 * What the chat's agents are doing right now, at the end of the transcript (where their reply
 * will land): a working avatar, a shimmering label and, past a few seconds, the elapsed time.
 * The label updates in place without animation; only `shimmer` signals that work is live.
 * In a DM the agent isn't named (the header already says who it is).
 */
export function ActivityIndicator({ agents, named }: { agents: Agent[]; named: boolean }) {
  const shown = useShownWorkers(agents)
  return (
    <div role="status" aria-live="polite" className="flex flex-col gap-1">
      {shown.map((agent) => (
        <WorkingRow key={agent.id} agent={agent} named={named} />
      ))}
    </div>
  )
}

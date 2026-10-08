import { SquareIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { activityLabel, useStopAgent } from "@/features/agents"
import type { Agent } from "@/lib/api-client"

/**
 * "openbot is thinking…" above the composer while agents in this chat are working, with a way to
 * stop each. The text updates in place without animation; only `shimmer` signals that work is
 * live.
 */
export function ActivityLine({ agents }: { agents: Agent[] }) {
  const working = agents
    .map((agent) => ({ agent, label: activityLabel(agent) }))
    .filter((entry): entry is { agent: Agent; label: string } => entry.label !== null)

  return (
    <div className="flex h-6 items-center gap-2 px-1 text-xs text-muted-foreground">
      <p className="min-w-0 flex-1 truncate" aria-live="polite">
        {working.map(({ agent, label }, i) => (
          <span key={agent.id}>
            {i > 0 && " · "}
            <span className="font-medium text-foreground/80">{agent.name}</span>{" "}
            <span className="shimmer">is {label}</span>
          </span>
        ))}
      </p>
      {working.map(({ agent }) => (
        <StopButton key={agent.id} agent={agent} named={working.length > 1} />
      ))}
    </div>
  )
}

/** "Stop" for one working agent; with several, each shows its name instead. */
function StopButton({ agent, named }: { agent: Agent; named: boolean }) {
  const stop = useStopAgent(agent.id)
  const stopping = stop.isPending || agent.activity.label === "stopping"
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="xs"
            className="-my-1 text-muted-foreground"
            disabled={stopping}
            aria-label={`Stop ${agent.name}`}
            onClick={() => stop.mutate()}
          />
        }
      >
        <SquareIcon className="fill-current" aria-hidden="true" />
        {named ? agent.name : "Stop"}
      </TooltipTrigger>
      <TooltipContent>Stop what {agent.name} is doing now</TooltipContent>
    </Tooltip>
  )
}

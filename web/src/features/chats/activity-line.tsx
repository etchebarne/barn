import { activityLabel } from "@/features/agents"
import type { Agent } from "@/lib/api-client"

/**
 * "barn is thinking…" above the composer while agents in this chat are working. The text
 * updates in place without animation; only `shimmer` signals that work is live.
 */
export function ActivityLine({ agents }: { agents: Agent[] }) {
  const working = agents
    .map((agent) => ({ agent, label: activityLabel(agent) }))
    .filter((entry): entry is { agent: Agent; label: string } => entry.label !== null)

  return (
    <div className="flex h-6 items-center px-1 text-xs text-muted-foreground" aria-live="polite">
      {working.length > 0 && (
        <p className="truncate">
          {working.map(({ agent, label }, i) => (
            <span key={agent.id}>
              {i > 0 && " · "}
              <span className="font-medium text-foreground/80">{agent.name}</span>{" "}
              <span className="shimmer">is {label}</span>
            </span>
          ))}
        </p>
      )}
    </div>
  )
}

import type { Agent } from "@/lib/api-client"

/** "thinking…", "running npm test…"; `null` when the agent is idle. */
export function activityLabel(agent: Pick<Agent, "activity">): string | null {
  if (agent.activity.state !== "working") return null
  const label = agent.activity.label?.trim() || "thinking"
  return label.endsWith("…") || label.endsWith("...") ? label : `${label}…`
}

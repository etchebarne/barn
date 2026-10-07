import { useQuery } from "@tanstack/react-query"

import { Checkbox } from "@/components/ui/checkbox"
import { agentsQueryOptions } from "@/lib/agents"

/** Checkboxes for which agents may use a connection. */
export function AgentAccess({
  value,
  onToggle,
  disabled,
  idPrefix,
}: {
  value: string[]
  onToggle: (agentId: string, on: boolean) => void
  disabled?: boolean
  idPrefix: string
}) {
  const { data: agents } = useQuery(agentsQueryOptions)
  if (!agents || agents.length === 0) {
    return <p className="text-sm text-muted-foreground">No agents yet.</p>
  }
  return (
    <ul className="flex flex-col gap-1" aria-label="Agents that can use this connection">
      {agents.map((agent) => {
        const id = `${idPrefix}-agent-${agent.id}`
        return (
          <li key={agent.id}>
            <label
              htmlFor={id}
              className="flex cursor-pointer items-center gap-3 rounded-md px-1 py-1.5 text-sm select-none hover:bg-muted/50"
            >
              <Checkbox
                id={id}
                aria-label={agent.name}
                checked={value.includes(agent.id)}
                disabled={disabled}
                onCheckedChange={(checked: boolean) => onToggle(agent.id, checked)}
              />
              {agent.name}
            </label>
          </li>
        )
      })}
    </ul>
  )
}

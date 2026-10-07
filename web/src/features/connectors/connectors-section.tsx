import { useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { agentsQueryOptions } from "@/lib/agents"
import type { Agent } from "@/lib/api-client"

import { useConnectors, useConnectorTypes } from "./api"
import { ConnectorIcon } from "./connector-icon"
import { ConnectorSheet } from "./connector-sheet"
import { usedByLabel } from "./logic"

/**
 * Settings → Connectors: connected accounts and "Add connection". `selection` ("new" or a
 * connection id) lives in the URL so the agent sheet can link to a connection.
 */
export function ConnectorsSection({
  selection,
  onSelect,
}: {
  selection: string | undefined
  onSelect: (selection: string | undefined) => void
}) {
  const { data: connectors, isPending, error } = useConnectors()
  const { data: types } = useConnectorTypes()
  const { data: agentList } = useQuery(agentsQueryOptions)
  const agents = new Map<string, Agent>((agentList ?? []).map((a) => [a.id, a]))

  return (
    <section
      id="connectors"
      className="flex scroll-mt-4 flex-col gap-4 border-b py-6 last:border-b-0"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-medium">Connectors</h2>
          <p className="text-sm text-muted-foreground">
            Apps your agents can use and react to, like Slack or GitHub.
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={() => onSelect("new")}>
          <PlusIcon />
          Add connection
        </Button>
      </div>
      {isPending ? (
        <Skeleton className="h-14 w-full rounded-lg" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load connections: {error.message}</p>
      ) : connectors.length === 0 ? (
        <p className="text-sm text-muted-foreground">No connections yet.</p>
      ) : (
        <ul className="flex flex-col gap-1" aria-label="Connections">
          {connectors.map((connector) => (
            <li key={connector.id}>
              <button
                type="button"
                className="flex w-full items-center gap-3 rounded-lg p-2 text-left outline-none select-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/50"
                onClick={() => onSelect(connector.id)}
              >
                <ConnectorIcon type={connector.type} />
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm font-medium">
                    {connector.name}
                    <span className="font-normal text-muted-foreground">
                      {" · "}
                      {types?.find((t) => t.type === connector.type)?.name ?? connector.type}
                    </span>
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {usedByLabel(connector.agentIds, agents)}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      <ConnectorSheet selection={selection} onSelect={onSelect} />
    </section>
  )
}

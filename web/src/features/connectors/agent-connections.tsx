import { Link } from "@tanstack/react-router"

import { Skeleton } from "@/components/ui/skeleton"

import { useConnectors, useConnectorTypes } from "./api"
import { ConnectorIcon } from "./connector-icon"

/** "Connected apps" in the agent sheet: accounts this agent may use, linking to their detail. */
export function AgentConnections({
  agentId,
  agentName,
  onNavigate,
}: {
  agentId: string
  agentName: string
  /** Called before following a link (e.g. to close the agent sheet). */
  onNavigate?: () => void
}) {
  const { data: connectors, isPending, error } = useConnectors()
  const { data: types } = useConnectorTypes()
  const mine = connectors?.filter((c) => c.agentIds.includes(agentId)) ?? []

  return (
    <section className="flex flex-col gap-2" aria-labelledby={`apps-${agentId}`}>
      <h3 id={`apps-${agentId}`} className="text-sm font-medium">
        Connected apps
      </h3>
      {isPending ? (
        <Skeleton className="h-10 w-full rounded-lg" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load apps: {error.message}</p>
      ) : mine.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No apps yet. Connect them in{" "}
          <Link
            to="/settings"
            hash="connectors"
            className="underline underline-offset-4 hover:text-foreground"
            onClick={onNavigate}
          >
            Settings → Connectors
          </Link>
          .
        </p>
      ) : (
        <ul className="flex flex-col gap-1" aria-label={`Apps ${agentName} can use`}>
          {mine.map((connector) => (
            <li key={connector.id}>
              <Link
                to="/settings"
                search={{ connector: connector.id }}
                className="flex items-center gap-3 rounded-lg p-1.5 text-sm select-none hover:bg-muted/50"
                onClick={onNavigate}
              >
                <ConnectorIcon type={connector.type} className="size-7" />
                <span className="truncate font-medium">{connector.name}</span>
                <span className="truncate text-muted-foreground">
                  {types?.find((t) => t.type === connector.type)?.name ?? connector.type}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

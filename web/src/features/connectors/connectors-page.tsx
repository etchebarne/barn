import { useQuery } from "@tanstack/react-query"
import { cn } from "cn"
import { CheckIcon, ChevronDownIcon, PlusIcon, SearchIcon, TriangleAlertIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { PageHeader } from "@/components/page-header"
import { SegmentedControl } from "@/components/segmented-control"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { useIsMobile } from "@/hooks/use-mobile"
import { agentsQueryOptions } from "@/lib/agents"
import type { Agent } from "@/lib/api-client"

import { useConnectors, useConnectorTypes } from "./api"
import { ConnectorIcon } from "./connector-icon"
import { ConnectorSheet } from "./connector-sheet"
import {
  addSelection,
  connectionApp,
  customTypes,
  discoverEntries,
  isConnected,
  matchesQuery,
  type DiscoverEntry,
} from "./discover"
import { usedByLabel, type Connector } from "./logic"
import { catalogQueryOptions } from "./signin"

type View = "yours" | "discover"

const CARD =
  "group/card relative flex h-full min-h-26 w-full items-start gap-3.5 rounded-xl border bg-card p-4 text-left outline-none select-none transition-[scale,border-color] duration-(--duration-press) ease-out-quint hover:border-foreground/15 focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.99]"

function Section({
  title,
  count,
  children,
}: {
  title: string
  count?: number
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-3" aria-label={title}>
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        {title}
        {count !== undefined && (
          <span className="rounded-md bg-muted px-1.5 text-xs font-medium text-muted-foreground tabular-nums">
            {count}
          </span>
        )}
      </h2>
      <ul className="grid gap-3 sm:grid-cols-2">{children}</ul>
    </section>
  )
}

/** A Discover card: logo, name, what it does, how it connects; + or ✓ in the corner. */
function DiscoverCard({
  entry,
  connected,
  onAdd,
}: {
  entry: DiscoverEntry
  connected: boolean
  onAdd: () => void
}) {
  return (
    <li>
      <button
        type="button"
        className={CARD}
        aria-label={`${connected ? "Add another" : "Connect"} ${entry.name}`}
        onClick={onAdd}
      >
        <ConnectorIcon type={entry.logo} size="lg" />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5 pr-8">
          <span className="truncate text-sm font-medium">{entry.name}</span>
          <span className="line-clamp-2 text-[13px] leading-[18px] text-muted-foreground">
            {entry.description}
          </span>
          <span className="mt-1 text-xs text-muted-foreground/80">{entry.method}</span>
        </span>
        <span
          aria-hidden="true"
          className={cn(
            "absolute top-3.5 right-3.5 flex size-7 items-center justify-center rounded-lg [&_svg]:size-4",
            connected
              ? "bg-success/15 text-success"
              : "bg-muted text-muted-foreground group-hover/card:bg-primary group-hover/card:text-primary-foreground",
          )}
        >
          {connected ? <CheckIcon /> : <PlusIcon />}
        </span>
      </button>
    </li>
  )
}

/** A connection of yours: logo, name, what it is, who can use it, and trouble if any. */
function ConnectionCard({
  connector,
  logo,
  kind,
  agents,
  onOpen,
}: {
  connector: Connector
  logo: string
  kind: string
  agents: Map<string, Agent>
  onOpen: () => void
}) {
  const expired = connector.signIn === "expired"
  return (
    <li>
      <button type="button" className={CARD} onClick={onOpen}>
        <ConnectorIcon type={logo} size="lg" />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="truncate text-sm font-medium">{connector.name}</span>
          {kind !== connector.name && (
            <span className="truncate text-[13px] text-muted-foreground">{kind}</span>
          )}
          <span className="mt-1 truncate text-xs text-muted-foreground/80">
            {usedByLabel(connector.agentIds, agents)}
          </span>
        </span>
        {expired && (
          <span className="absolute top-3.5 right-3.5 flex items-center gap-1 rounded-full bg-warning-soft px-2 py-0.5 text-xs font-medium text-warning-foreground">
            <TriangleAlertIcon className="size-3" aria-hidden="true" />
            Reconnect
          </span>
        )}
      </button>
    </li>
  )
}

function GridSkeleton() {
  return (
    <div className="grid gap-3 sm:grid-cols-2" aria-hidden="true">
      {Array.from({ length: 4 }, (_, i) => (
        <Skeleton key={i} className="h-26 rounded-xl" />
      ))}
    </div>
  )
}

/**
 * Connectors: your connections and the apps you can connect, as cards with the apps' logos.
 * Adding and each connection's details open in one sheet (`connector` in the URL: "new",
 * "new:app:<id>", "new:type:<type>", or a connection id).
 */
export function ConnectorsPage({
  connector,
  onConnectorChange,
}: {
  connector: string | undefined
  onConnectorChange: (connector: string | undefined) => void
}) {
  const mobile = useIsMobile()
  const connectors = useConnectors()
  const types = useConnectorTypes()
  const catalog = useQuery(catalogQueryOptions)
  const { data: agentList } = useQuery(agentsQueryOptions)
  const agents = new Map<string, Agent>((agentList ?? []).map((a) => [a.id, a]))
  const [query, setQuery] = useState("")
  // Your connections when there are some; otherwise straight to what you could connect.
  const [chosenView, setView] = useState<View | null>(null)
  const yoursCount = connectors.data?.length ?? 0
  const view: View = chosenView ?? (connectors.isPending || yoursCount > 0 ? "yours" : "discover")

  const entries = discoverEntries(catalog.data, types.data).filter((e) =>
    matchesQuery(query, e.name, e.description),
  )
  const custom = customTypes(types.data)
  const shownCustom = custom.filter((t) => matchesQuery(query, t.name, t.description))
  const mine = (connectors.data ?? []).flatMap((c) => {
    const app = connectionApp(c, catalog.data)
    const typeName = types.data?.find((t) => t.type === c.type)?.name ?? c.type
    const kind = app ? `${app.name} · signed in` : typeName
    return matchesQuery(query, c.name, kind)
      ? [{ connector: c, logo: app?.id ?? c.type, kind }]
      : []
  })
  const loading = types.isPending || catalog.isPending

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
      {mobile && (
        <PageHeader>
          <h1 className="truncate text-sm font-semibold">Connectors</h1>
        </PageHeader>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-8 px-4 pt-4 pb-16 md:px-8 md:pt-10">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
            {!mobile && <h1 className="text-2xl font-semibold tracking-tight">Connectors</h1>}
            <SegmentedControl
              label="Connections"
              value={view}
              onChange={setView}
              options={[
                { value: "yours", label: "Yours", count: yoursCount },
                { value: "discover", label: "Discover" },
              ]}
            />
            <div className="ml-auto flex w-full items-center gap-2 sm:w-auto">
              <label className="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-lg border bg-surface px-2.5 text-sm focus-within:border-foreground/20 sm:w-56 sm:flex-none">
                <SearchIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <input
                  type="search"
                  aria-label="Search connectors"
                  placeholder="Search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
                />
              </label>
              <DropdownMenu>
                <DropdownMenuTrigger render={<Button size="sm" className="h-8 gap-1.5 px-3" />}>
                  <PlusIcon />
                  Add
                  <ChevronDownIcon className="-mr-0.5 opacity-60" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-72">
                  <DropdownMenuItem onClick={() => onConnectorChange("new")}>
                    <SearchIcon />
                    Browse all apps…
                  </DropdownMenuItem>
                  {custom.length > 0 && <DropdownMenuLabel>Your own</DropdownMenuLabel>}
                  {custom.map((type) => (
                    <DropdownMenuItem
                      key={type.type}
                      className="items-start"
                      onClick={() =>
                        onConnectorChange(addSelection({ kind: "type", id: type.type }))
                      }
                    >
                      <ConnectorIcon type={type.type} className="size-7" />
                      <span className="flex min-w-0 flex-col">
                        {type.name}
                        <span className="line-clamp-2 text-xs text-muted-foreground">
                          {type.description}
                        </span>
                      </span>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          {view === "yours" ? (
            connectors.isPending ? (
              <GridSkeleton />
            ) : connectors.error ? (
              <p className="text-sm text-destructive">
                Couldn't load connections: {connectors.error.message}
              </p>
            ) : yoursCount === 0 ? (
              <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed px-6 py-14 text-center">
                <p className="text-[15px] font-medium">No connections yet</p>
                <p className="max-w-80 text-[13px] text-muted-foreground">
                  Connect apps like Slack, GitHub or Notion so your agents can use them and react to
                  what happens there.
                </p>
                <Button size="sm" variant="outline" onClick={() => setView("discover")}>
                  Browse apps
                </Button>
              </div>
            ) : mine.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nothing matches “{query}”.</p>
            ) : (
              <Section title="Connected" count={mine.length}>
                {mine.map(({ connector: c, logo, kind }) => (
                  <ConnectionCard
                    key={c.id}
                    connector={c}
                    logo={logo}
                    kind={kind}
                    agents={agents}
                    onOpen={() => onConnectorChange(c.id)}
                  />
                ))}
              </Section>
            )
          ) : loading ? (
            <GridSkeleton />
          ) : entries.length === 0 && shownCustom.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nothing matches “{query}”.</p>
          ) : (
            <>
              {entries.length > 0 && (
                <Section title="Apps" count={entries.length}>
                  {entries.map((entry) => (
                    <DiscoverCard
                      key={`${entry.kind}:${entry.id}`}
                      entry={entry}
                      connected={isConnected(entry, connectors.data, catalog.data)}
                      onAdd={() => onConnectorChange(addSelection(entry))}
                    />
                  ))}
                </Section>
              )}
              {shownCustom.length > 0 && (
                <Section title="Your own" count={shownCustom.length}>
                  {shownCustom.map((type) => (
                    <DiscoverCard
                      key={type.type}
                      entry={{
                        kind: "type",
                        id: type.type,
                        logo: type.type,
                        name: type.name,
                        description: type.description,
                        method: "Any server you run or trust",
                      }}
                      connected={false}
                      onAdd={() => onConnectorChange(addSelection({ kind: "type", id: type.type }))}
                    />
                  ))}
                </Section>
              )}
            </>
          )}
        </div>
      </div>
      <ConnectorSheet selection={connector} onSelect={onConnectorChange} />
    </div>
  )
}

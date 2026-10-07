import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"

import { AgentAccess } from "./agent-access"
import { ConnectorIcon } from "./connector-icon"
import { toggleGrant } from "./logic"
import { SignInButton } from "./sign-in-button"
import { catalogQueryOptions, type CatalogApp, type SignInDeps } from "./signin"

const ROW =
  "flex w-full items-center gap-3 rounded-xl border p-2.5 text-left transition-transform duration-(--duration-press) ease-out-quint outline-none select-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.99]"

/** "Sign in with": apps openbot connects to with a sign-in instead of API keys. */
export function CatalogList({ onPick }: { onPick: (app: CatalogApp) => void }) {
  const { data: apps, isPending, error } = useQuery(catalogQueryOptions)
  if (isPending) return <Skeleton className="h-14 w-full rounded-xl" />
  if (error) return <p className="text-sm text-destructive">Couldn't load apps: {error.message}</p>
  if (apps.length === 0) return null
  return (
    <ul className="flex flex-col gap-2" aria-label="Sign in with">
      {apps.map((app) => (
        <li key={app.id}>
          <button type="button" className={ROW} onClick={() => onPick(app)}>
            <ConnectorIcon type={app.id} />
            <span className="flex min-w-0 flex-col">
              <span className="text-sm font-medium">{app.name}</span>
              <span className="truncate text-sm text-muted-foreground">{app.description}</span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

/** The short step before signing in: name and which agents can use it. */
export function CatalogSignIn({ app, deps }: { app: CatalogApp; deps?: SignInDeps }) {
  const [name, setName] = useState(app.name)
  const [agentIds, setAgentIds] = useState<string[]>([])
  return (
    <div className="flex flex-col gap-6">
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`signin-name-${app.id}`}>Name</FieldLabel>
          <Input
            id={`signin-name-${app.id}`}
            autoComplete="off"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
      </FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">Agents that can use it</FieldLegend>
        <AgentAccess
          idPrefix={`signin-${app.id}`}
          value={agentIds}
          onToggle={(agentId, on) => setAgentIds((ids) => toggleGrant(ids, agentId, on))}
        />
      </FieldSet>
      <SignInButton
        appName={app.name}
        deps={deps}
        request={() => ({ url: app.url, name: name.trim() || app.name, agentIds })}
      />
      <p className="text-xs text-muted-foreground">
        You'll go to {app.name} to allow access, then come back here.
      </p>
    </div>
  )
}

import { useQuery } from "@tanstack/react-query"
import { ArrowLeftIcon, CircleCheckIcon } from "lucide-react"
import { useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSet,
  FieldLegend,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"

import { AgentAccess } from "./agent-access"
import { useConnectorTypes, useCreateConnector } from "./api"
import { CatalogList, CatalogSignIn } from "./catalog"
import { ConnectorFields } from "./connector-fields"
import { ConnectorIcon } from "./connector-icon"
import type { AddStart } from "./discover"
import {
  buildCreateRequest,
  buildFormFields,
  connectFormFields,
  signInUrl,
  initialValues,
  toggleGrant,
  validateForm,
  type Connector,
  type ConnectorType,
} from "./logic"
import { SetupSteps } from "./setup-steps"
import { SignInButton } from "./sign-in-button"
import { catalogQueryOptions, needsSignIn, type CatalogApp } from "./signin"
import { SignalsList, WebhookInfo, WebhookSecret } from "./webhook-info"

/** Types that share a name with a sign-in app connect with a key instead; say so. */
export function apiKeyNote(type: ConnectorType, catalog: CatalogApp[] | undefined): string | null {
  const same = catalog?.some((app) => app.name.toLowerCase() === type.name.toLowerCase())
  return same ? "API key · needed for webhooks and events" : null
}

function TypePicker({
  onPick,
  onPickApp,
}: {
  onPick: (type: ConnectorType) => void
  onPickApp: (app: CatalogApp) => void
}) {
  return (
    <div className="flex flex-col gap-6">
      <section aria-labelledby="sign-in-with" className="flex flex-col gap-2">
        <h3 id="sign-in-with" className="text-sm font-medium">
          Sign in with
        </h3>
        <CatalogList onPick={onPickApp} />
      </section>
      <section aria-labelledby="other-apps" className="flex flex-col gap-2">
        <h3 id="other-apps" className="text-sm font-medium">
          Other apps
        </h3>
        <TypeList onPick={onPick} />
      </section>
    </div>
  )
}

function TypeList({ onPick }: { onPick: (type: ConnectorType) => void }) {
  const { data: catalog } = useQuery(catalogQueryOptions)
  const { data: types, isPending, error } = useConnectorTypes()
  if (isPending) {
    return (
      <div className="flex flex-col gap-2">
        <Skeleton className="h-16 w-full rounded-xl" />
        <Skeleton className="h-16 w-full rounded-xl" />
      </div>
    )
  }
  if (error) return <p className="text-sm text-destructive">Couldn't load apps: {error.message}</p>
  return (
    <ul className="flex flex-col gap-2" aria-label="Apps you can connect">
      {types.map((type) => (
        <li key={type.type}>
          <button
            type="button"
            className="flex w-full items-start gap-3 rounded-xl border p-3 text-left transition-transform duration-(--duration-press) ease-out-quint outline-none select-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.99]"
            onClick={() => onPick(type)}
          >
            <ConnectorIcon type={type.type} />
            <span className="flex min-w-0 flex-col gap-0.5">
              <span className="text-sm font-medium">{type.name}</span>
              <span className="text-sm text-muted-foreground">{type.description}</span>
              {apiKeyNote(type, catalog) && (
                <span className="text-xs text-muted-foreground">{apiKeyNote(type, catalog)}</span>
              )}
            </span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function ConnectForm({
  type,
  onConnected,
}: {
  type: ConnectorType
  onConnected: (connector: Connector) => void
}) {
  const create = useCreateConnector()
  // Webhook signing secrets aren't needed to connect; they're set under "Receive events".
  const [fields] = useState(() => connectFormFields(buildFormFields(type)))
  const [values, setValues] = useState(() => initialValues(fields))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [name, setName] = useState(type.name)
  const [agentIds, setAgentIds] = useState<string[]>([])

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const next = validateForm(fields, values)
    setErrors(next)
    if (Object.keys(next).length > 0) return
    create.mutate(buildCreateRequest(type, fields, values, name, agentIds), {
      onSuccess: (connector) => {
        toast.success(`Connected ${connector.name}`)
        onConnected(connector)
      },
    })
  }

  if (needsSignIn(create.error)) {
    const url = signInUrl(fields, values)
    return (
      <div className="flex flex-col gap-3">
        <p className="text-sm font-medium">This server uses sign-in</p>
        <p className="text-sm text-muted-foreground">
          Instead of a key, you'll allow access on the server's own page, then come back here.
        </p>
        <SignInButton
          label="Sign in"
          appName={name.trim() || type.name}
          request={() => ({ url, name: name.trim() || type.name, agentIds })}
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="w-fit"
          onClick={() => create.reset()}
        >
          Back to the form
        </Button>
      </div>
    )
  }

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-6">
      {type.setup.steps.length > 0 && (
        <section aria-labelledby="how-to-connect" className="flex flex-col gap-3">
          <h3 id="how-to-connect" className="text-sm font-medium">
            How to connect
          </h3>
          <SetupSteps steps={type.setup.steps} label={`How to connect ${type.name}`} />
        </section>
      )}
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="new-connector-name">Name</FieldLabel>
          <Input
            id="new-connector-name"
            autoComplete="off"
            value={name}
            disabled={create.isPending}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <ConnectorFields
          idPrefix="new-connector"
          fields={fields}
          values={values}
          errors={errors}
          disabled={create.isPending}
          onChange={(id, value) => {
            setValues((v) => ({ ...v, [id]: value }))
            setErrors((e) => ({ ...e, [id]: "" }))
            if (create.error) create.reset()
          }}
        />
      </FieldGroup>
      <FieldSet>
        <FieldLegend variant="label">Agents that can use it</FieldLegend>
        <AgentAccess
          idPrefix="new-connector"
          value={agentIds}
          disabled={create.isPending}
          onToggle={(agentId, on) => setAgentIds((ids) => toggleGrant(ids, agentId, on))}
        />
      </FieldSet>
      {create.error && !needsSignIn(create.error) && (
        <FieldError>{create.error.message}</FieldError>
      )}
      <Button type="submit" disabled={create.isPending}>
        {create.isPending && <Spinner />}
        {create.isPending ? `Checking with ${type.name}…` : "Connect"}
      </Button>
    </form>
  )
}

export function Connected({
  connector,
  type,
  onDone,
  onOpen,
}: {
  connector: Connector
  type: ConnectorType
  onDone: () => void
  onOpen: () => void
}) {
  return (
    <div className="flex flex-col gap-6">
      <p className="flex items-center gap-2 text-sm">
        <CircleCheckIcon className="size-4 shrink-0" aria-hidden="true" />
        {connector.name} is connected.
      </p>
      {type.setup.eventSteps.length > 0 && (
        <p className="text-sm text-muted-foreground">
          To let agents react to {type.name} events, follow the steps under Receive events in the
          connection's settings.
        </p>
      )}
      <WebhookInfo connector={connector} type={type} />
      <WebhookSecret connector={connector} type={type} />
      <SignalsList type={type} />
      <div className="flex justify-end gap-2">
        <Button variant="outline" onClick={onOpen}>
          View connection
        </Button>
        <Button onClick={onDone}>Done</Button>
      </div>
    </div>
  )
}

export type AddStep =
  | { step: "type" }
  | { step: "signin"; app: CatalogApp }
  | { step: "form"; type: ConnectorType }
  | { step: "done"; type: ConnectorType; connector: Connector }

/** Where `start` points, once the catalog and types are loaded (null until then). */
function startStep(
  start: AddStart | null,
  catalog: CatalogApp[] | undefined,
  types: ConnectorType[] | undefined,
): AddStep | null {
  if (!start) return { step: "type" }
  if (start.kind === "app") {
    if (!catalog) return null
    const app = catalog.find((a) => a.id === start.id)
    return app ? { step: "signin", app } : { step: "type" }
  }
  if (!types) return null
  const type = types.find((t) => t.type === start.id)
  return type ? { step: "form", type } : { step: "type" }
}

/**
 * The add flow, stepping in place inside one sheet: type → form → done, with Back. `start`
 * skips the picker (a card on the Connectors page was clicked).
 */
export function AddConnection({
  start = null,
  onDone,
  onOpen,
  onStepChange,
}: {
  start?: AddStart | null
  onDone: () => void
  onOpen: (connectorId: string) => void
  onStepChange?: (step: AddStep["step"]) => void
}) {
  const { data: catalog } = useQuery(catalogQueryOptions)
  const { data: types } = useConnectorTypes()
  const [chosen, setChosen] = useState<AddStep | null>(null)
  const state = chosen ?? startStep(start, catalog, types)
  function go(next: AddStep) {
    setChosen(next)
    onStepChange?.(next.step)
  }

  if (!state) return <Skeleton className="h-40 w-full rounded-xl" />

  /** The API-key type for an app that also offers sign-in (Linear), if any. */
  function keyType(app: CatalogApp): ConnectorType | undefined {
    return types?.find((t) => t.name.toLowerCase() === app.name.toLowerCase())
  }

  if (state.step === "type") {
    return (
      <TypePicker
        onPick={(type) => go({ step: "form", type })}
        onPickApp={(app) => go({ step: "signin", app })}
      />
    )
  }
  if (state.step === "signin") {
    return (
      <div className="flex flex-col gap-4">
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2 w-fit"
          onClick={() => go({ step: "type" })}
        >
          <ArrowLeftIcon />
          Back
        </Button>
        <div className="flex items-center gap-3">
          <ConnectorIcon type={state.app.id} />
          <div className="flex min-w-0 flex-col">
            <span className="text-sm font-medium">{state.app.name}</span>
            <span className="text-sm text-muted-foreground">{state.app.description}</span>
          </div>
        </div>
        <CatalogSignIn app={state.app} />
        {keyType(state.app) && (
          <button
            type="button"
            className="w-fit text-left text-xs text-muted-foreground underline underline-offset-3 hover:text-foreground"
            onClick={() => {
              const type = keyType(state.app)
              if (type) go({ step: "form", type })
            }}
          >
            Or connect {state.app.name} with an API key instead (also gets events)
          </button>
        )}
      </div>
    )
  }
  if (state.step === "form") {
    return (
      <div className="flex flex-col gap-4">
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2 w-fit"
          onClick={() => go({ step: "type" })}
        >
          <ArrowLeftIcon />
          Back
        </Button>
        <div className="flex items-center gap-3">
          <ConnectorIcon type={state.type.type} />
          <div className="flex min-w-0 flex-col">
            <span className="text-sm font-medium">{state.type.name}</span>
            <span className="text-sm text-muted-foreground">{state.type.description}</span>
          </div>
        </div>
        <ConnectForm
          type={state.type}
          onConnected={(connector) => go({ step: "done", type: state.type, connector })}
        />
      </div>
    )
  }
  return (
    <Connected
      connector={state.connector}
      type={state.type}
      onDone={onDone}
      onOpen={() => onOpen(state.connector.id)}
    />
  )
}

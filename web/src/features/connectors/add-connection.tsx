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
import { ConnectorFields } from "./connector-fields"
import { ConnectorIcon } from "./connector-icon"
import {
  buildCreateRequest,
  buildFormFields,
  connectFormFields,
  initialValues,
  toggleGrant,
  validateForm,
  type Connector,
  type ConnectorType,
} from "./logic"
import { SetupSteps } from "./setup-steps"
import { SignalsList, WebhookInfo, WebhookSecret } from "./webhook-info"

function TypePicker({ onPick }: { onPick: (type: ConnectorType) => void }) {
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
      {create.error && <FieldError>{create.error.message}</FieldError>}
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
  | { step: "form"; type: ConnectorType }
  | { step: "done"; type: ConnectorType; connector: Connector }

/** The add flow, stepping in place inside one sheet: type → form → done, with Back. */
export function AddConnection({
  onDone,
  onOpen,
  onStepChange,
}: {
  onDone: () => void
  onOpen: (connectorId: string) => void
  onStepChange?: (step: AddStep["step"]) => void
}) {
  const [state, setState] = useState<AddStep>({ step: "type" })
  function go(next: AddStep) {
    setState(next)
    onStepChange?.(next.step)
  }

  if (state.step === "type") {
    return <TypePicker onPick={(type) => go({ step: "form", type })} />
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

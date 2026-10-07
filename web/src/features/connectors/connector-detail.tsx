import { useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

import { AgentAccess } from "./agent-access"
import { useDeleteConnector, useSetConnectorAgents, useUpdateConnector } from "./api"
import { ConnectorFields } from "./connector-fields"
import {
  buildFormFields,
  buildUpdateRequest,
  initialValues,
  toggleGrant,
  validateForm,
  type Connector,
  type ConnectorType,
} from "./logic"
import { SignalsList, WebhookInfo } from "./webhook-info"

function NameField({ connector }: { connector: Connector }) {
  const update = useUpdateConnector(connector.id)
  const [draft, setDraft] = useState<string | null>(null)

  function save() {
    if (draft === null) return
    const name = draft.trim()
    if (!name || name === connector.name) {
      setDraft(null)
      return
    }
    update.mutate(
      { name },
      {
        onSuccess: (updated) => {
          setDraft(null)
          toast.success(`Renamed to ${updated.name}`)
        },
      },
    )
  }

  return (
    <Field data-invalid={!!update.error || undefined}>
      <FieldLabel htmlFor={`connector-name-${connector.id}`}>Name</FieldLabel>
      <Input
        id={`connector-name-${connector.id}`}
        autoComplete="off"
        value={draft ?? connector.name}
        disabled={update.isPending}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={save}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault()
            save()
          } else if (e.key === "Escape" && draft !== null) {
            e.preventDefault()
            e.stopPropagation()
            setDraft(null)
          }
        }}
      />
      {update.error && <FieldError>{update.error.message}</FieldError>}
    </Field>
  )
}

function CredentialsForm({ connector, type }: { connector: Connector; type: ConnectorType }) {
  const update = useUpdateConnector(connector.id)
  const fields = buildFormFields(type, connector)
  const [values, setValues] = useState(() => initialValues(fields, connector))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const pristine = initialValues(fields, connector)
  const dirty = Object.keys(values).some((k) => values[k] !== pristine[k])

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const next = validateForm(fields, values)
    setErrors(next)
    if (Object.keys(next).length > 0) return
    update.mutate(buildUpdateRequest(fields, values), {
      onSuccess: (updated) => {
        setValues(initialValues(buildFormFields(type, updated), updated))
        toast.success(`Updated ${updated.name}`)
      },
    })
  }

  if (fields.length === 0) return null
  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      <FieldGroup>
        <ConnectorFields
          idPrefix={`connector-${connector.id}`}
          fields={fields}
          values={values}
          errors={errors}
          disabled={update.isPending}
          onChange={(id, value) => {
            setValues((v) => ({ ...v, [id]: value }))
            setErrors((e) => ({ ...e, [id]: "" }))
            if (update.error) update.reset()
          }}
        />
      </FieldGroup>
      {update.error && <FieldError>{update.error.message}</FieldError>}
      {dirty && (
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={update.isPending}
            onClick={() => {
              setValues(pristine)
              setErrors({})
              update.reset()
            }}
          >
            Discard
          </Button>
          <Button type="submit" size="sm" disabled={update.isPending}>
            {update.isPending && <Spinner />}
            {update.isPending ? `Checking with ${type.name}…` : "Save"}
          </Button>
        </div>
      )}
    </form>
  )
}

function Access({ connector }: { connector: Connector }) {
  const setAgents = useSetConnectorAgents(connector.id)
  return (
    <section className="flex flex-col gap-2" aria-labelledby={`access-${connector.id}`}>
      <h3 id={`access-${connector.id}`} className="text-sm font-medium">
        Agents that can use it
      </h3>
      <AgentAccess
        idPrefix={`connector-${connector.id}`}
        value={connector.agentIds}
        onToggle={(agentId, on) =>
          setAgents.mutate(toggleGrant(connector.agentIds, agentId, on), {
            onError: (e) => toast.error(`Couldn't update access: ${e.message}`),
          })
        }
      />
    </section>
  )
}

function Disconnect({
  connector,
  onDisconnected,
}: {
  connector: Connector
  onDisconnected: () => void
}) {
  const remove = useDeleteConnector(connector.id)
  const [confirming, setConfirming] = useState(false)
  return (
    <section className="flex flex-col gap-3" aria-labelledby={`disconnect-${connector.id}`}>
      <h3 id={`disconnect-${connector.id}`} className="text-sm font-medium text-destructive">
        Danger zone
      </h3>
      {confirming ? (
        <div
          role="alertdialog"
          aria-label={`Disconnect ${connector.name}?`}
          className="flex flex-col gap-3 rounded-[calc(var(--radius-md)+0.75rem)] border border-destructive/30 bg-destructive/5 p-3 text-sm"
        >
          <div className="flex flex-col gap-1">
            <p className="font-medium">Disconnect {connector.name}?</p>
            <p className="text-muted-foreground">
              Agents lose access, and its saved credentials and the tasks that react to its events
              are deleted.
            </p>
          </div>
          {remove.error && <FieldError>{remove.error.message}</FieldError>}
          <div className="flex justify-end gap-2">
            <Button
              size="sm"
              variant="ghost"
              autoFocus
              disabled={remove.isPending}
              onClick={() => setConfirming(false)}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(undefined, {
                  onSuccess: () => {
                    toast.success(`Disconnected ${connector.name}`)
                    onDisconnected()
                  },
                })
              }
            >
              {remove.isPending && <Spinner />}
              Disconnect
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-4 rounded-[calc(var(--radius-md)+0.75rem)] border border-destructive/30 p-3">
          <p className="text-sm text-muted-foreground">
            Remove this connection, its credentials and any tasks that wait on its events.
          </p>
          <Button variant="destructive" size="sm" onClick={() => setConfirming(true)}>
            Disconnect
          </Button>
        </div>
      )}
    </section>
  )
}

/** One connected account: rename, credentials and config, agent access, webhook, disconnect. */
export function ConnectorDetail({
  connector,
  type,
  onDisconnected,
}: {
  connector: Connector
  type: ConnectorType | undefined
  onDisconnected: () => void
}) {
  return (
    <div className="flex flex-col gap-8">
      <FieldGroup>
        <NameField connector={connector} />
      </FieldGroup>
      {type && <CredentialsForm connector={connector} type={type} />}
      <Access connector={connector} />
      <WebhookInfo connector={connector} type={type} />
      <SignalsList type={type} />
      <Disconnect connector={connector} onDisconnected={onDisconnected} />
    </div>
  )
}

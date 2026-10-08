import { KeyRoundIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useId, useRef, useState, type FormEvent } from "react"
import { toast } from "sonner"

import { SecretInput } from "@/components/secret-input"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { useEscapeCancel } from "@/hooks/use-escape-cancel"
import type { Agent } from "@/lib/api-client"

import { SECRET_NAME, useDeleteSecret, useSecrets, useSetSecret, type AgentSecret } from "./api"
import { formatRelative } from "./schedule"

/** What a name field turns typing into: capitals, with spaces and dashes as underscores. */
export function toSecretName(input: string): string {
  return input.toUpperCase().replace(/[\s-]/g, "_")
}

/** Why a name isn't valid yet, or null. */
export function secretNameError(name: string): string | null {
  if (!name) return "Enter a name."
  if (!/^[A-Z]/.test(name)) return "Start with a letter."
  if (!/^[A-Z0-9_]+$/.test(name)) return "Use capital letters, digits and _ only."
  if (!SECRET_NAME.test(name)) return "Use 2 to 64 characters."
  return null
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : "Something went wrong."
}

/** Add a secret: name (upper-cased as you type), optional description, and the value. */
function AddSecretForm({
  existing,
  onSave,
  onCancel,
}: {
  existing: AgentSecret[]
  onSave: (name: string, value: string, description: string) => Promise<void>
  onCancel: () => void
}) {
  const id = useId()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [value, setValue] = useState("")
  const [touched, setTouched] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameProblem = secretNameError(name)
  const replaces = existing.some((s) => s.name === name)
  const formRef = useRef<HTMLFormElement>(null)
  useEscapeCancel(formRef, onCancel)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setTouched(true)
    if (nameProblem || !value.trim() || pending) return
    setPending(true)
    setError(null)
    try {
      await onSave(name, value, description.trim())
      setValue("")
    } catch (e) {
      setError(message(e))
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      ref={formRef}
      aria-label="Add secret"
      noValidate
      className="flex flex-col gap-3 rounded-[calc(var(--radius-lg)+0.75rem)] border p-3"
      onSubmit={(e) => void submit(e)}
    >
      <Field data-invalid={(touched && !!nameProblem) || undefined}>
        <FieldLabel htmlFor={`${id}-name`}>Name</FieldLabel>
        <Input
          id={`${id}-name`}
          autoFocus
          spellCheck={false}
          autoComplete="off"
          maxLength={64}
          placeholder="GITHUB_TOKEN"
          className="font-mono"
          value={name}
          aria-invalid={(touched && !!nameProblem) || undefined}
          onChange={(event) => setName(toSecretName(event.target.value))}
          onBlur={() => setTouched(true)}
        />
        {touched && nameProblem ? (
          <FieldError>{nameProblem}</FieldError>
        ) : (
          <FieldDescription>
            {replaces
              ? `Replaces the current value of ${name}.`
              : "Capital letters, digits and _, starting with a letter."}
          </FieldDescription>
        )}
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-description`}>Description (optional)</FieldLabel>
        <Input
          id={`${id}-description`}
          placeholder="What it is and where it's from"
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-value`}>Value</FieldLabel>
        <SecretInput
          autoComplete="off"
          className="font-mono"
          id={`${id}-value`}
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </Field>
      {error && <FieldError>{error}</FieldError>}
      <div className="flex justify-end gap-2">
        <Button type="button" size="sm" variant="ghost" disabled={pending} onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={pending || !value.trim() || !!nameProblem}>
          {pending && <Spinner />}
          Save
        </Button>
      </div>
    </form>
  )
}

/** One secret: replace its value inline, or delete it after an inline confirm. */
function SecretRow({
  secret,
  now,
  onReplace,
  onDelete,
}: {
  secret: AgentSecret
  now: Date
  onReplace: (value: string) => Promise<void>
  onDelete: () => void
}) {
  const [mode, setMode] = useState<"view" | "replace" | "delete">("view")
  const [value, setValue] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function replace(event: FormEvent) {
    event.preventDefault()
    if (!value.trim() || pending) return
    setPending(true)
    setError(null)
    try {
      await onReplace(value)
      setValue("")
      setMode("view")
    } catch (e) {
      setError(message(e))
    } finally {
      setPending(false)
    }
  }

  function cancel() {
    setValue("")
    setError(null)
    setMode("view")
  }
  const replaceRef = useRef<HTMLFormElement>(null)
  useEscapeCancel(replaceRef, cancel)

  return (
    <li className="group/secret flex flex-col gap-2 rounded-lg bg-muted/50 py-2 pr-1.5 pl-3 text-sm">
      <div className="flex items-start gap-2">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="font-mono text-xs font-medium break-all">{secret.name}</span>
          {secret.description && (
            <span className="wrap-break-word text-muted-foreground">{secret.description}</span>
          )}
          <span className="text-xs text-muted-foreground">
            Updated {formatRelative(secret.updatedAt, now)}
          </span>
        </div>
        {mode === "view" && (
          <div className="flex shrink-0 items-center gap-1 opacity-100 sm:opacity-0 sm:group-focus-within/secret:opacity-100 sm:group-hover/secret:opacity-100">
            <Button
              variant="ghost"
              size="xs"
              aria-label={`Replace ${secret.name}`}
              onClick={() => setMode("replace")}
            >
              Replace
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Delete ${secret.name}`}
              className="text-muted-foreground hover:text-destructive"
              onClick={() => setMode("delete")}
            >
              <Trash2Icon />
            </Button>
          </div>
        )}
        {mode === "delete" && (
          <div
            role="alertdialog"
            aria-label={`Delete ${secret.name}?`}
            className="flex shrink-0 items-center gap-1"
          >
            <Button variant="ghost" size="xs" autoFocus onClick={() => setMode("view")}>
              Cancel
            </Button>
            <Button variant="destructive" size="xs" onClick={onDelete}>
              Delete
            </Button>
          </div>
        )}
      </div>
      {mode === "replace" && (
        <form
          ref={replaceRef}
          aria-label={`Replace ${secret.name}`}
          className="flex flex-col gap-2"
          onSubmit={(e) => void replace(e)}
        >
          <SecretInput
            autoComplete="off"
            className="font-mono"
            aria-label={`New value for ${secret.name}`}
            autoFocus
            placeholder="New value"
            value={value}
            onChange={(event) => setValue(event.target.value)}
          />
          {error && <FieldError>{error}</FieldError>}
          <div className="flex justify-end gap-2">
            <Button type="button" size="xs" variant="ghost" disabled={pending} onClick={cancel}>
              Cancel
            </Button>
            <Button type="submit" size="xs" disabled={pending || !value.trim()}>
              {pending && <Spinner />}
              Save
            </Button>
          </div>
        </form>
      )}
    </li>
  )
}

/**
 * The agent's secrets (API keys, tokens), available as environment variables in its computer.
 * Values are write-only: they can be replaced, never shown.
 */
export function SecretsSection({ agent }: { agent: Agent }) {
  const { data: secrets, isPending, error } = useSecrets(agent.id)
  const setSecret = useSetSecret(agent.id)
  const remove = useDeleteSecret(agent.id)
  const [adding, setAdding] = useState(false)
  const [openedAt] = useState(() => new Date())

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-secrets">
      <div className="flex items-center justify-between gap-2">
        <h3 id="agent-secrets" className="text-sm font-medium">
          Secrets
        </h3>
        {secrets && !adding && (
          <Button variant="ghost" size="xs" onClick={() => setAdding(true)}>
            <PlusIcon />
            Add secret
          </Button>
        )}
      </div>
      <p className="text-sm text-muted-foreground">
        Available to {agent.name} as environment variables in its computer. Values are never shown.
      </p>
      {adding && secrets && (
        <AddSecretForm
          existing={secrets}
          onCancel={() => setAdding(false)}
          onSave={async (name, value, description) => {
            await setSecret(name, value, description)
            setAdding(false)
            toast.success(`Saved ${name}`)
          }}
        />
      )}
      {isPending ? (
        <Skeleton className="h-12 w-full rounded-lg" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load secrets: {error.message}</p>
      ) : secrets.length === 0 ? (
        !adding && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <KeyRoundIcon className="size-4 shrink-0" aria-hidden="true" />
            No secrets yet. {agent.name} asks for one when it needs it, or add one here.
          </p>
        )
      ) : (
        <ul className="flex flex-col gap-1" aria-label="Secrets">
          {secrets.map((secret) => (
            <SecretRow
              key={secret.name}
              secret={secret}
              now={openedAt}
              onReplace={(value) => setSecret(secret.name, value)}
              onDelete={() =>
                remove.mutate(secret.name, {
                  onSuccess: () => toast.success(`Deleted ${secret.name}`),
                  onError: (e) => toast.error(`Couldn't delete ${secret.name}: ${e.message}`),
                })
              }
            />
          ))}
        </ul>
      )}
    </section>
  )
}

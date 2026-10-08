import { CheckIcon, KeyRoundIcon, XIcon } from "lucide-react"
import { useState, type FormEvent } from "react"

import { LinkifiedText } from "@/components/linkified-text"
import { SecretInput } from "@/components/secret-input"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { ApiError } from "@/lib/api-client"

import type { Prompt } from "./logic"

/** Not now = option 1 (option 0, Save, is only reachable by providing the value). */
export const DECLINE_SECRET = { selected: [1] }

export function secretOutcome(prompt: Prompt): "saved" | "declined" | null {
  if (prompt.kind !== "secret" || prompt.status !== "answered") return null
  return prompt.answer?.selected?.[0] === 0 ? "saved" : "declined"
}

/** The value lives only in this form's state, and is cleared once it's saved. */
function SecretForm({
  agentName,
  name,
  disabled,
  onSave,
  onDecline,
}: {
  agentName: string
  name: string
  disabled: boolean
  onSave: (value: string) => Promise<void>
  onDecline: () => void
}) {
  const [value, setValue] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const busy = disabled || pending
  const empty = value.trim() === ""

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (empty || busy) return
    setPending(true)
    setError(null)
    try {
      await onSave(value)
      setValue("")
    } catch (e) {
      setError(
        e instanceof ApiError || e instanceof Error ? e.message : "Couldn't save it. Try again.",
      )
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      onSubmit={(e) => void submit(e)}
      noValidate
      aria-label={`Provide ${name}`}
      className="flex flex-col gap-2 px-1.5 pb-1"
    >
      <SecretInput
        autoComplete="off"
        className="font-mono"
        aria-label={name}
        placeholder="Paste the value"
        value={value}
        disabled={busy}
        aria-invalid={!!error || undefined}
        onChange={(event) => {
          setValue(event.target.value)
          setError(null)
        }}
      />
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        Stored encrypted. {agentName} uses it in its computer but never sees the value.
      </p>
      <div className="flex justify-end gap-2">
        <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={onDecline}>
          Not now
        </Button>
        <Button type="submit" size="sm" disabled={busy || empty}>
          {pending && <Spinner />}
          Save
        </Button>
      </div>
    </form>
  )
}

/**
 * An agent asking for a secret (an API key, a token): the variable name, what it is and where
 * to get it, and a password field. The value goes straight to the server and is never shown
 * again; the card then reads "Saved as NAME".
 */
export function SecretCard({
  prompt,
  agentName,
  isLatest,
  disabled,
  onSave,
  onDecline,
  onDismiss,
}: {
  prompt: Prompt
  agentName: string
  isLatest: boolean
  disabled: boolean
  onSave: (value: string) => Promise<void>
  onDecline: () => void
  onDismiss: () => void
}) {
  const name = prompt.secret?.name ?? "SECRET"
  const description = prompt.secret?.description ?? ""
  const pending = prompt.status === "pending"
  const outcome = secretOutcome(prompt)

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex items-center gap-2.5 px-1.5 pt-1">
        <span
          aria-hidden="true"
          className="flex size-7 shrink-0 items-center justify-center rounded-md border bg-background"
        >
          <KeyRoundIcon className="size-3.5" />
        </span>
        <p
          className={
            pending ? "min-w-0 flex-1 font-medium" : "min-w-0 flex-1 text-muted-foreground"
          }
        >
          {agentName} needs a secret
        </p>
        {pending && isLatest && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Dismiss question"
            className="text-muted-foreground"
            disabled={disabled}
            onClick={onDismiss}
          >
            <XIcon />
          </Button>
        )}
      </div>
      <div className="flex flex-col gap-1 px-1.5">
        <code className="w-fit rounded-md border bg-background/60 px-1.5 py-0.5 font-mono text-xs break-all">
          {name}
        </code>
        {description && (
          <p className="text-sm wrap-break-word text-muted-foreground">
            <LinkifiedText text={description} />
          </p>
        )}
      </div>
      {pending && (
        <SecretForm
          agentName={agentName}
          name={name}
          disabled={disabled}
          onSave={onSave}
          onDecline={onDecline}
        />
      )}
      {outcome === "saved" && (
        <p className="flex items-center gap-1.5 px-1.5 pb-1 text-xs font-medium">
          <CheckIcon className="size-3.5" aria-hidden="true" />
          Saved as <span className="font-mono">{name}</span>
        </p>
      )}
      {outcome === "declined" && (
        <p className="flex items-center gap-1.5 px-1.5 pb-1 text-xs font-medium text-muted-foreground">
          <XIcon className="size-3.5" aria-hidden="true" />
          Not provided
        </p>
      )}
    </div>
  )
}

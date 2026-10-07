import { useState, type FormEvent, type ReactNode } from "react"

import { SecretInput } from "@/components/secret-input"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"

import { useUpdateProviderKey } from "./provider-api"

export const OPENCODE_GO_DOCS_URL = "https://opencode.ai/docs/go/"

/** API key entry for OpenCode Go, shared by onboarding and Settings. */
export function ProviderKeyForm({
  submitLabel,
  onSaved,
  secondaryAction,
  autoFocus,
}: {
  submitLabel: string
  onSaved?: () => void
  secondaryAction?: ReactNode
  autoFocus?: boolean
}) {
  const update = useUpdateProviderKey()
  const [apiKey, setApiKey] = useState("")

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const trimmed = apiKey.trim()
    if (!trimmed) return
    update.mutate(trimmed, {
      onSuccess: () => {
        setApiKey("")
        onSaved?.()
      },
    })
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4">
      <Field data-invalid={!!update.error || undefined}>
        <FieldLabel htmlFor="provider-api-key">OpenCode Go API key</FieldLabel>
        <SecretInput
          id="provider-api-key"
          autoComplete="off"
          autoFocus={autoFocus}
          placeholder="Paste your key"
          value={apiKey}
          aria-invalid={!!update.error || undefined}
          onChange={(e) => {
            setApiKey(e.target.value)
            if (update.error) update.reset()
          }}
        />
        {update.error ? (
          <FieldError>{update.error.message}</FieldError>
        ) : (
          <FieldDescription>
            Get a key from your{" "}
            <a href={OPENCODE_GO_DOCS_URL} target="_blank" rel="noreferrer">
              OpenCode Go account
            </a>
            . It's checked with OpenCode Go, then stored encrypted on your server.
          </FieldDescription>
        )}
      </Field>
      <div className="flex items-center justify-end gap-2">
        {secondaryAction}
        <Button type="submit" disabled={update.isPending || !apiKey.trim()}>
          {update.isPending && <Spinner />}
          {update.isPending ? "Checking key…" : submitLabel}
        </Button>
      </div>
    </form>
  )
}

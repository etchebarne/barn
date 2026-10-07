import { useState, type FormEvent } from "react"

import { ModelPicker } from "@/components/model-picker"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

import { DEFAULT_AGENT_NAME, useCompleteOnboarding } from "./api"

export function AgentStep({ onComplete }: { onComplete: (chatId: string) => void }) {
  const complete = useCompleteOnboarding()
  const [name, setName] = useState(DEFAULT_AGENT_NAME)
  const [model, setModel] = useState<string | null>(null)
  const [errors, setErrors] = useState<{ name?: string; model?: string }>({})

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const next: typeof errors = {}
    if (!name.trim()) next.name = "Give your agent a name."
    else if (name.trim().length > 64) next.name = "Use at most 64 characters."
    if (!model) next.model = "Pick a model."
    setErrors(next)
    if (next.name || next.model || !model) return
    complete.mutate(
      { model, agentName: name.trim() },
      { onSuccess: ({ chatId }) => onComplete(chatId) },
    )
  }

  return (
    <form onSubmit={onSubmit} noValidate>
      <FieldGroup>
        <Field data-invalid={!!errors.name || undefined}>
          <FieldLabel htmlFor="agent-name">Name</FieldLabel>
          <Input
            id="agent-name"
            autoComplete="off"
            value={name}
            aria-invalid={!!errors.name || undefined}
            onChange={(e) => setName(e.target.value)}
          />
          {errors.name ? (
            <FieldError>{errors.name}</FieldError>
          ) : (
            <FieldDescription>You can rename it later by asking it.</FieldDescription>
          )}
        </Field>
        <Field data-invalid={!!errors.model || undefined}>
          <FieldLabel htmlFor="agent-model">Model</FieldLabel>
          <ModelPicker
            id="agent-model"
            value={model}
            invalid={!!errors.model}
            onValueChange={(value) => {
              setModel(value)
              if (value) setErrors((e) => ({ ...e, model: undefined }))
            }}
          />
          <FieldError>{errors.model}</FieldError>
        </Field>
        {complete.error && <FieldError>{complete.error.message}</FieldError>}
        <Button type="submit" size="lg" disabled={complete.isPending}>
          {complete.isPending && <Spinner />}
          Create agent
        </Button>
      </FieldGroup>
    </form>
  )
}

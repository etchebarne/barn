import { useState } from "react"
import { toast } from "sonner"

import { ModelPicker } from "@/components/model-picker"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import type { Agent } from "@/lib/api-client"

import { AgentAvatar } from "./agent-avatar"
import { useUpdateAgent } from "./api"

/** Model setting: changes apply as soon as a model is picked. */
function ModelField({ agent }: { agent: Agent }) {
  const update = useUpdateAgent(agent.id)
  // While saving (or after a failed save) show the chosen model; otherwise the server's.
  const [choice, setChoice] = useState<string | null>(null)
  const value = choice ?? agent.model

  function onValueChange(model: string | null) {
    if (!model || model === agent.model) {
      setChoice(null)
      update.reset()
      return
    }
    setChoice(model)
    update.mutate(
      { model },
      {
        onSuccess: (updated) => {
          setChoice(null)
          toast.success(`${updated.name} now uses ${updated.model}`)
        },
      },
    )
  }

  return (
    <Field data-invalid={!!update.error || undefined}>
      <FieldLabel htmlFor="agent-details-model" className="gap-2">
        Model
        {update.isPending && <Spinner className="size-3.5 text-muted-foreground" />}
      </FieldLabel>
      <ModelPicker
        id="agent-details-model"
        value={value}
        invalid={!!update.error}
        disabled={update.isPending}
        onValueChange={onValueChange}
      />
      {update.error ? (
        <FieldError>{update.error.message}</FieldError>
      ) : (
        <FieldDescription>Used for every model call this agent makes.</FieldDescription>
      )}
    </Field>
  )
}

/**
 * Agent details in a side sheet, opened from the chat header. One section per setting so more
 * fields (language, notifications, trust mode) slot in later.
 */
export function AgentDetailsSheet({
  agent,
  open,
  onOpenChange,
}: {
  agent: Agent
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md"
      >
        <SheetHeader className="flex-row items-center gap-3 border-b pr-12">
          <AgentAvatar name={agent.name} size="lg" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <SheetTitle className="truncate">{agent.name}</SheetTitle>
            <SheetDescription>Agent settings</SheetDescription>
          </div>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
          <FieldGroup>
            {/* Key by agent so switching agents never shows another agent's pending choice. */}
            <ModelField key={agent.id} agent={agent} />
          </FieldGroup>
        </div>
      </SheetContent>
    </Sheet>
  )
}

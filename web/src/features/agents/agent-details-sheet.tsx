import { useRef, useState, type Ref } from "react"
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
import { useAgentsById, useUpdateAgent } from "./api"
import { useAgentDetailsStore } from "./details-store"

/** Model setting: changes apply as soon as a model is picked. */
function ModelField({ agent, inputRef }: { agent: Agent; inputRef?: Ref<HTMLInputElement> }) {
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
        inputRef={inputRef}
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
 * Agent details in a side sheet, opened from the chat header or a failure notice (see
 * `openAgentDetails`). Mounted once. One section per setting so more fields (language,
 * notifications, trust mode) slot in later.
 */
export function AgentDetailsSheet() {
  const agentId = useAgentDetailsStore((s) => s.agentId)
  const focus = useAgentDetailsStore((s) => s.focus)
  const close = useAgentDetailsStore((s) => s.close)
  const agent = useAgentsById().get(agentId ?? "")
  // Keep showing the last agent while the sheet animates closed.
  const [shown, setShown] = useState<Agent | undefined>(agent)
  if (agent && agent !== shown) setShown(agent)
  const modelInputRef = useRef<HTMLInputElement>(null)

  return (
    <Sheet
      open={agentId !== null && agent !== undefined}
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      {shown && (
        <SheetContent
          side="right"
          className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md"
          initialFocus={focus === "model" ? modelInputRef : undefined}
        >
          <SheetHeader className="flex-row items-center gap-3 border-b pr-12">
            <AgentAvatar name={shown.name} size="lg" />
            <div className="flex min-w-0 flex-col gap-0.5">
              <SheetTitle className="truncate">{shown.name}</SheetTitle>
              <SheetDescription>Agent settings</SheetDescription>
            </div>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
            <FieldGroup>
              {/* Key by agent so switching agents never shows another agent's pending choice. */}
              <ModelField key={shown.id} agent={shown} inputRef={modelInputRef} />
            </FieldGroup>
          </div>
        </SheetContent>
      )}
    </Sheet>
  )
}

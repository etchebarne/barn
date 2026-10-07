import { EraserIcon } from "lucide-react"
import { useState, type KeyboardEvent, type ReactNode } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { Agent } from "@/lib/api-client"
import { clearHistoryCopy, useAgentDm, useClearChatHistory } from "@/lib/chat-history"

import { useDeleteAgent, useUpdateAgent } from "./api"
import { AUTO_LANGUAGE, isAutoLanguage, trustChangeNeedsConfirmation } from "./trust"

/**
 * Inline confirmation inside the sheet. Confirmations live here rather than in a dialog so a
 * dialog never stacks on top of the sheet.
 */
function InlineConfirm({
  tone,
  title,
  children,
  confirmLabel,
  pending,
  error,
  onConfirm,
  onCancel,
}: {
  tone: "warning" | "destructive"
  title: string
  children: ReactNode
  confirmLabel: string
  pending: boolean
  error?: string | null
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div
      role="alertdialog"
      aria-label={title}
      className={
        tone === "warning"
          ? "flex flex-col gap-3 rounded-[calc(var(--radius-md)+0.75rem)] border border-warning/40 bg-warning/10 p-3 text-sm"
          : "flex flex-col gap-3 rounded-[calc(var(--radius-md)+0.75rem)] border border-destructive/30 bg-destructive/5 p-3 text-sm"
      }
    >
      <div className="flex flex-col gap-1">
        <p className="font-medium">{title}</p>
        <p className="text-muted-foreground">{children}</p>
      </div>
      {error && <FieldError>{error}</FieldError>}
      <div className="flex justify-end gap-2">
        <Button size="sm" variant="ghost" disabled={pending} onClick={onCancel} autoFocus>
          Cancel
        </Button>
        <Button
          size="sm"
          variant={tone === "destructive" ? "destructive" : "default"}
          disabled={pending}
          onClick={onConfirm}
        >
          {pending && <Spinner />}
          {confirmLabel}
        </Button>
      </div>
    </div>
  )
}

/** Name: edited inline, saved on blur or Enter; Escape reverts. */
export function NameSection({ agent }: { agent: Agent }) {
  const update = useUpdateAgent(agent.id)
  const [draft, setDraft] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const value = draft ?? agent.name

  function save() {
    if (draft === null) return
    const name = draft.trim()
    if (name === agent.name) {
      setDraft(null)
      return
    }
    if (!name) {
      setError("The name can't be empty.")
      return
    }
    if (name.length > 64) {
      setError("Use at most 64 characters.")
      return
    }
    setError(null)
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

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault()
      save()
    } else if (event.key === "Escape" && draft !== null) {
      // Revert instead of closing the sheet.
      event.preventDefault()
      event.stopPropagation()
      setDraft(null)
      setError(null)
      update.reset()
    }
  }

  const shownError = error ?? update.error?.message
  return (
    <Field data-invalid={!!shownError || undefined}>
      <FieldLabel htmlFor="agent-name-field" className="gap-2">
        Name
        {update.isPending && <Spinner className="size-3.5 text-muted-foreground" />}
      </FieldLabel>
      <Input
        id="agent-name-field"
        autoComplete="off"
        value={value}
        maxLength={64}
        aria-invalid={!!shownError || undefined}
        disabled={update.isPending}
        onChange={(e) => {
          setDraft(e.target.value)
          setError(null)
        }}
        onBlur={save}
        onKeyDown={onKeyDown}
      />
      {shownError && <FieldError>{shownError}</FieldError>}
    </Field>
  )
}

/** A growing textarea for one of an agent's long texts, with an explicit Save once edited. */
function LongTextSection({
  agent,
  field,
  label,
  placeholder,
  description,
}: {
  agent: Agent
  field: "instructions" | "personality"
  label: string
  placeholder: string
  description: string
}) {
  const update = useUpdateAgent(agent.id)
  const [draft, setDraft] = useState<string | null>(null)
  const value = draft ?? agent[field]
  const dirty = draft !== null && draft !== agent[field]
  const id = `agent-${field}`

  function save() {
    if (!dirty) return
    update.mutate(
      { [field]: draft },
      {
        onSuccess: (updated) => {
          setDraft(null)
          toast.success(`Saved ${updated.name}'s ${label.toLowerCase()}`)
        },
      },
    )
  }

  return (
    <Field data-invalid={!!update.error || undefined}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Textarea
        id={id}
        value={value}
        placeholder={placeholder}
        className="max-h-72 min-h-24 overscroll-contain"
        aria-invalid={!!update.error || undefined}
        onChange={(e) => {
          setDraft(e.target.value)
          if (update.error) update.reset()
        }}
      />
      {update.error ? (
        <FieldError>{update.error.message}</FieldError>
      ) : (
        <FieldDescription>{description}</FieldDescription>
      )}
      {dirty && (
        <div className="flex justify-end gap-2">
          <Button
            size="sm"
            variant="ghost"
            disabled={update.isPending}
            onClick={() => {
              setDraft(null)
              update.reset()
            }}
          >
            Discard
          </Button>
          <Button size="sm" disabled={update.isPending} onClick={save}>
            {update.isPending && <Spinner />}
            Save
          </Button>
        </div>
      )}
    </Field>
  )
}

/** Instructions: what the agent does. */
export function InstructionsSection({ agent }: { agent: Agent }) {
  return (
    <LongTextSection
      agent={agent}
      field="instructions"
      label="Instructions"
      placeholder="What this agent is for and how it should work"
      description="Its own system instructions, used on every turn."
    />
  )
}

/** Personality: how the agent comes across, apart from what it does and what it remembers. */
export function PersonalitySection({ agent }: { agent: Agent }) {
  return (
    <LongTextSection
      agent={agent}
      field="personality"
      label="Personality"
      placeholder="Warm and playful, lots of emoji. Or: dry, terse, all lowercase."
      description="How it comes across: tone, voice, humour. It can change this itself when you ask it to talk differently."
    />
  )
}

/** Reply language: auto (match the user) or a specific language, saved on blur or Enter. */
export function LanguageSection({ agent }: { agent: Agent }) {
  const update = useUpdateAgent(agent.id)
  const auto = isAutoLanguage(agent.language)
  // "specific" picked but no language saved yet.
  const [choosing, setChoosing] = useState(false)
  const [draft, setDraft] = useState<string | null>(null)
  const mode = auto && !choosing ? "auto" : "specific"
  const value = draft ?? (auto ? "" : agent.language)

  function saveLanguage(language: string) {
    update.mutate(
      { language },
      {
        onSuccess: (updated) => {
          setDraft(null)
          setChoosing(false)
          toast.success(
            isAutoLanguage(updated.language)
              ? `${updated.name} now replies in your language`
              : `${updated.name} now replies in ${updated.language}`,
          )
        },
      },
    )
  }

  function saveDraft() {
    if (draft === null) return
    const language = draft.trim()
    if (!language) {
      setDraft(null)
      return
    }
    if (language === agent.language) {
      setDraft(null)
      return
    }
    saveLanguage(language)
  }

  return (
    <Field data-invalid={!!update.error || undefined}>
      <FieldLabel className="gap-2">
        Reply language
        {update.isPending && <Spinner className="size-3.5 text-muted-foreground" />}
      </FieldLabel>
      <ToggleGroup
        variant="outline"
        aria-label="Reply language"
        value={[mode]}
        disabled={update.isPending}
        onValueChange={(next: unknown[]) => {
          if (next[0] === "auto") {
            setChoosing(false)
            setDraft(null)
            if (!auto) saveLanguage(AUTO_LANGUAGE)
          } else if (next[0] === "specific") {
            setChoosing(true)
          }
        }}
      >
        <ToggleGroupItem value="auto" className="select-none">
          Auto (match the user)
        </ToggleGroupItem>
        <ToggleGroupItem value="specific" className="select-none">
          Specific language
        </ToggleGroupItem>
      </ToggleGroup>
      {mode === "specific" && (
        <Input
          aria-label="Language"
          placeholder="e.g. Spanish"
          autoFocus={choosing && auto}
          value={value}
          disabled={update.isPending}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={saveDraft}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              saveDraft()
            }
          }}
        />
      )}
      {update.error && <FieldError>{update.error.message}</FieldError>}
    </Field>
  )
}

/** Trusted mode: turning it on asks first; turning it off doesn't. */
export function TrustSection({ agent }: { agent: Agent }) {
  const update = useUpdateAgent(agent.id)
  const [confirming, setConfirming] = useState(false)
  const trusted = agent.trustMode === "trusted"

  function apply(trustMode: Agent["trustMode"]) {
    update.mutate(
      { trustMode },
      {
        onSuccess: (updated) => {
          setConfirming(false)
          toast.success(
            updated.trustMode === "trusted"
              ? `${updated.name} is now trusted`
              : `${updated.name} will ask before gated actions`,
          )
        },
      },
    )
  }

  return (
    <Field data-invalid={(!!update.error && !confirming) || undefined}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-0.5">
          <FieldLabel htmlFor="agent-trusted">Trusted mode</FieldLabel>
          <FieldDescription>
            Skip approvals for actions like making changes in connected apps. Deleting an agent
            always asks.
          </FieldDescription>
        </div>
        {/* Stays off until the user confirms; Base UI's switch isn't a labelable element, so it
            gets its name from aria-label. */}
        <Switch
          id="agent-trusted"
          aria-label="Trusted mode"
          checked={trusted}
          disabled={update.isPending}
          onCheckedChange={(checked: boolean) => {
            const next = checked ? "trusted" : "ask"
            if (trustChangeNeedsConfirmation(agent.trustMode, next)) {
              update.reset()
              setConfirming(true)
            } else if (confirming) {
              setConfirming(false)
            } else {
              apply(next)
            }
          }}
        />
      </div>
      {confirming ? (
        <InlineConfirm
          tone="warning"
          title={`Trust ${agent.name}?`}
          confirmLabel="Trust"
          pending={update.isPending}
          error={update.error?.message}
          onConfirm={() => apply("trusted")}
          onCancel={() => {
            setConfirming(false)
            update.reset()
          }}
        >
          It'll take actions that normally need your approval, like making changes in connected
          apps, without asking. Deleting an agent still asks.
        </InlineConfirm>
      ) : (
        update.error && <FieldError>{update.error.message}</FieldError>
      )}
    </Field>
  )
}

/** Push notifications for this agent's messages. */
export function NotificationsSection({ agent }: { agent: Agent }) {
  const update = useUpdateAgent(agent.id)
  return (
    <Field data-invalid={!!update.error || undefined}>
      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-0.5">
          <FieldLabel htmlFor="agent-notifications">Notifications</FieldLabel>
          <FieldDescription>
            Push {agent.name}'s messages to devices with notifications on (see Settings).
          </FieldDescription>
        </div>
        <Switch
          id="agent-notifications"
          checked={agent.notifications}
          disabled={update.isPending}
          onCheckedChange={(checked: boolean) =>
            update.mutate(
              { notifications: checked },
              {
                onSuccess: (updated) =>
                  toast.success(
                    updated.notifications
                      ? `You'll be notified when ${updated.name} messages you`
                      : `Notifications from ${updated.name} are off`,
                  ),
              },
            )
          }
        />
      </div>
      {update.error && <FieldError>{update.error.message}</FieldError>}
    </Field>
  )
}

/** Danger zone: clear the DM's history, or delete the agent for good, each confirmed inline. */
export function DangerZone({ agent }: { agent: Agent }) {
  const remove = useDeleteAgent(agent)
  const dm = useAgentDm(agent.id)
  const clear = useClearChatHistory({ id: dm?.id ?? "", name: agent.name })
  const clearCopy = clearHistoryCopy(agent.name)
  const [confirming, setConfirming] = useState<"clear" | "delete" | null>(null)
  const row =
    "flex items-center justify-between gap-4 rounded-[calc(var(--radius-md)+0.75rem)] border border-destructive/30 p-3"

  return (
    <section aria-labelledby="agent-danger-zone" className="flex flex-col gap-3">
      <h3 id="agent-danger-zone" className="text-sm font-medium text-destructive">
        Danger zone
      </h3>
      {confirming === "clear" && dm ? (
        <InlineConfirm
          tone="destructive"
          title={clearCopy.title}
          confirmLabel="Clear history"
          pending={clear.isPending}
          onConfirm={() => clear.mutate(undefined, { onSuccess: () => setConfirming(null) })}
          onCancel={() => {
            setConfirming(null)
            clear.reset()
          }}
        >
          {clearCopy.body}
        </InlineConfirm>
      ) : confirming === "delete" ? (
        <InlineConfirm
          tone="destructive"
          title={`Delete ${agent.name}? This can't be undone.`}
          confirmLabel="Delete"
          pending={remove.isPending}
          error={remove.error?.message}
          onConfirm={() => remove.mutate()}
          onCancel={() => {
            setConfirming(null)
            remove.reset()
          }}
        >
          Removes {agent.name}, its DM, memories, tasks and files for good. Messages it sent in
          group chats stay.
        </InlineConfirm>
      ) : (
        <>
          {dm && (
            <div className={row}>
              <p className="text-sm text-muted-foreground">
                Delete your DM's messages and files. {agent.name} forgets the conversation.
              </p>
              <Button variant="destructive" size="sm" onClick={() => setConfirming("clear")}>
                <EraserIcon />
                Clear history
              </Button>
            </div>
          )}
          <div className={row}>
            <p className="text-sm text-muted-foreground">
              Removes {agent.name}, its DM, memories, tasks and files for good. Messages it sent in
              group chats stay.
            </p>
            <Button variant="destructive" size="sm" onClick={() => setConfirming("delete")}>
              Delete agent
            </Button>
          </div>
        </>
      )}
    </section>
  )
}

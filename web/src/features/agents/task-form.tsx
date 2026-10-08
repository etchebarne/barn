import { cn } from "cn"
import { ChevronDownIcon } from "lucide-react"
import { useId, useRef, useState, type ComponentProps } from "react"

import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useEscapeCancel } from "@/hooks/use-escape-cancel"

import type { CreateTaskBody } from "./api"
import {
  defaultDraft,
  describeDraft,
  describeSchedule,
  draftFromTask,
  scheduleBody,
  type RepeatPreset,
  type ScheduleDraft,
  type Task,
} from "./schedule"

const PRESETS: { value: RepeatPreset; label: string }[] = [
  { value: "daily", label: "Every day" },
  { value: "weekdays", label: "Weekdays" },
  { value: "weekly", label: "Every week" },
  { value: "hourly", label: "Every hour" },
  { value: "custom", label: "Custom cron" },
]

const WEEKDAYS = Array.from({ length: 7 }, (_, day) =>
  // 2023-01-01 was a Sunday.
  new Intl.DateTimeFormat(undefined, { weekday: "long" }).format(new Date(2023, 0, 1 + day)),
)

/** A native select (best on phones), styled like the inputs. */
function Select({ className, children, ...props }: ComponentProps<"select">) {
  return (
    <div className={cn("relative", className)}>
      <select
        className="h-8 w-full min-w-0 appearance-none rounded-lg border border-input bg-transparent py-1 pr-8 pl-2.5 text-base transition-colors outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 md:text-sm dark:bg-input/30"
        {...props}
      >
        {children}
      </select>
      <ChevronDownIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 right-2.5 size-4 -translate-y-1/2 text-muted-foreground"
      />
    </div>
  )
}

function minuteOf(time: string): number {
  return Number(time.split(":")[1] ?? 0) || 0
}

/** Repeats: a preset plus its time, or a raw cron expression. */
function RepeatFields({
  draft,
  onChange,
}: {
  draft: ScheduleDraft
  onChange: (changes: Partial<ScheduleDraft>) => void
}) {
  const id = useId()
  return (
    <div className="flex flex-wrap gap-2">
      <Select
        aria-label="Repeats"
        className="min-w-36 flex-1"
        value={draft.preset}
        onChange={(event) => {
          const preset = PRESETS.find((p) => p.value === event.target.value)?.value ?? "daily"
          onChange({ preset })
        }}
      >
        {PRESETS.map((preset) => (
          <option key={preset.value} value={preset.value}>
            {preset.label}
          </option>
        ))}
      </Select>
      {draft.preset === "weekly" && (
        <Select
          aria-label="Day"
          className="min-w-32 flex-1"
          value={String(draft.weekday)}
          onChange={(event) => onChange({ weekday: Number(event.target.value) })}
        >
          {WEEKDAYS.map((name, day) => (
            <option key={name} value={day}>
              on {name}
            </option>
          ))}
        </Select>
      )}
      {(draft.preset === "daily" || draft.preset === "weekdays" || draft.preset === "weekly") && (
        <Input
          type="time"
          aria-label="Time"
          className="w-32"
          required
          value={draft.time}
          onChange={(event) => onChange({ time: event.target.value })}
        />
      )}
      {draft.preset === "hourly" && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <label htmlFor={`${id}-minute`}>at minute</label>
          <Input
            id={`${id}-minute`}
            type="number"
            min={0}
            max={59}
            className="w-20 tabular-nums"
            value={minuteOf(draft.time)}
            onChange={(event) => {
              const minute = Math.min(59, Math.max(0, Number(event.target.value) || 0))
              onChange({ time: `00:${String(minute).padStart(2, "0")}` })
            }}
          />
        </div>
      )}
      {draft.preset === "custom" && (
        <Input
          aria-label="Cron expression"
          placeholder="minute hour day month weekday, e.g. 30 9 * * 1"
          className="w-full font-mono"
          spellCheck={false}
          value={draft.cron}
          onChange={(event) => onChange({ cron: event.target.value })}
        />
      )}
    </div>
  )
}

/**
 * Creates or edits a task inline (no dialog over the sheet). Signal tasks keep their trigger:
 * only the name and purpose are editable, and the trigger shows read-only.
 */
export function TaskForm({
  task,
  pending,
  error,
  now,
  onSubmit,
  onCancel,
}: {
  task?: Task
  pending: boolean
  error: string | undefined
  now?: Date
  onSubmit: (body: CreateTaskBody) => void
  onCancel: () => void
}) {
  const id = useId()
  const [name, setName] = useState(task?.name ?? "")
  const [purpose, setPurpose] = useState(task?.purpose ?? "")
  const [draft, setDraft] = useState<ScheduleDraft>(() =>
    task ? draftFromTask(task, now) : defaultDraft(now),
  )
  const [check, setCheck] = useState(task?.check ?? "")
  const signal = task?.kind === "signal"
  const preview = signal ? null : describeDraft(draft)
  const canSave = name.trim() !== "" && purpose.trim() !== "" && !pending
  const update = (changes: Partial<ScheduleDraft>) => setDraft((d) => ({ ...d, ...changes }))

  // Escape anywhere in the form closes the form, not the sheet around it.
  const formRef = useRef<HTMLFormElement>(null)
  useEscapeCancel(formRef, onCancel)

  return (
    <form
      ref={formRef}
      aria-label={task ? `Edit ${task.name}` : "New task"}
      className="flex flex-col gap-3 rounded-[calc(var(--radius-lg)+0.75rem)] border p-3"
      onSubmit={(event) => {
        event.preventDefault()
        if (!canSave) return
        const base = { name: name.trim(), purpose: purpose.trim() }
        if (signal) return onSubmit(base)
        // New tasks send a check only when there is one; edits send it only when it changed
        // (an empty string removes it).
        const nextCheck = check.trim()
        const sendCheck = task ? nextCheck !== (task.check ?? "") : nextCheck !== ""
        onSubmit({ ...base, ...scheduleBody(draft), ...(sendCheck && { check: nextCheck }) })
      }}
    >
      <Field>
        <FieldLabel htmlFor={`${id}-name`}>Name</FieldLabel>
        <Input
          id={`${id}-name`}
          autoFocus
          required
          maxLength={80}
          placeholder="e.g. Weekday check-in"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-purpose`}>What it does</FieldLabel>
        <Textarea
          id={`${id}-purpose`}
          required
          className="min-h-16"
          placeholder="e.g. Give Martin a short catch-up on what changed since yesterday."
          value={purpose}
          onChange={(event) => setPurpose(event.target.value)}
        />
      </Field>
      {signal ? (
        <Field>
          <FieldLabel>Runs</FieldLabel>
          <FieldDescription>
            {describeSchedule(task)}. Its trigger can't be changed here; ask the agent instead.
          </FieldDescription>
        </Field>
      ) : (
        <Field>
          <FieldLabel id={`${id}-schedule`}>Schedule</FieldLabel>
          <ToggleGroup
            variant="outline"
            aria-labelledby={`${id}-schedule`}
            value={[draft.mode]}
            onValueChange={(next: unknown[]) => {
              if (next[0] === "repeat" || next[0] === "once") update({ mode: next[0] })
            }}
          >
            <ToggleGroupItem value="repeat" className="select-none">
              Repeats
            </ToggleGroupItem>
            <ToggleGroupItem value="once" className="select-none">
              Once
            </ToggleGroupItem>
          </ToggleGroup>
          {draft.mode === "repeat" ? (
            <RepeatFields draft={draft} onChange={update} />
          ) : (
            <div className="flex flex-wrap gap-2">
              <Input
                type="date"
                aria-label="Date"
                className="w-40"
                required
                value={draft.date}
                onChange={(event) => update({ date: event.target.value })}
              />
              <Input
                type="time"
                aria-label="Time"
                className="w-32"
                required
                value={draft.onceTime}
                onChange={(event) => update({ onceTime: event.target.value })}
              />
            </div>
          )}
          <FieldDescription aria-live="polite">
            {preview ??
              (draft.mode === "repeat" && draft.preset === "custom"
                ? "Not a valid cron expression yet."
                : "")}
          </FieldDescription>
        </Field>
      )}
      {!signal && (
        <Field>
          <FieldLabel htmlFor={`${id}-check`}>Only wake when this changes</FieldLabel>
          <Input
            id={`${id}-check`}
            className="font-mono"
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            placeholder="curl -s https://status.example.com | jq .state"
            value={check}
            onChange={(event) => setCheck(event.target.value)}
          />
          <FieldDescription>
            A command run on the schedule in the agent's computer. The agent is only woken when its
            output changes, so watching something costs nothing until there's news.
          </FieldDescription>
        </Field>
      )}
      {error && <FieldError>{error}</FieldError>}
      <div className="flex justify-end gap-2">
        <Button type="button" size="sm" variant="ghost" disabled={pending} onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={!canSave}>
          {pending && <Spinner />}
          {task ? "Save" : "Create task"}
        </Button>
      </div>
    </form>
  )
}

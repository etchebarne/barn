import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { FieldError } from "@/components/ui/field"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import type { Agent } from "@/lib/api-client"

import { useCreateMemory, useDeleteMemory, useMemories, useUpdateMemory, type Memory } from "./api"

const dateFormat = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" })

/** The server's limit (after trimming). */
export const MEMORY_MAX = 500
/** The counter appears this close to the limit. */
const COUNTER_FROM = MEMORY_MAX - 100

/**
 * Writes or rewrites a memory inline: Enter saves, Shift+Enter adds a line, Escape cancels
 * (without closing the sheet).
 */
function MemoryEditor({
  initial = "",
  label,
  pending,
  error,
  onSave,
  onCancel,
}: {
  initial?: string
  label: string
  pending: boolean
  error: string | undefined
  onSave: (text: string) => void
  onCancel: () => void
}) {
  const [text, setText] = useState(initial)
  const trimmed = text.trim()
  const canSave = trimmed.length > 0 && trimmed !== initial.trim() && !pending
  const save = () => {
    if (canSave) onSave(trimmed)
  }
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault()
        save()
      }}
    >
      <Textarea
        aria-label={label}
        autoFocus
        maxLength={MEMORY_MAX}
        value={text}
        aria-invalid={!!error || undefined}
        placeholder="e.g. Martin prefers short answers with the numbers first."
        className="min-h-16"
        onFocus={(event) => {
          // Start at the end when rewriting.
          const length = event.currentTarget.value.length
          event.currentTarget.setSelectionRange(length, length)
        }}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
            event.preventDefault()
            save()
          } else if (event.key === "Escape") {
            event.preventDefault()
            event.stopPropagation()
            onCancel()
          }
        }}
      />
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">{error && <FieldError>{error}</FieldError>}</div>
        {text.length >= COUNTER_FROM && (
          <span
            className="text-xs text-muted-foreground tabular-nums"
            aria-label={`${text.length} of ${MEMORY_MAX} characters`}
          >
            {text.length}/{MEMORY_MAX}
          </span>
        )}
        <Button type="button" size="sm" variant="ghost" disabled={pending} onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={!canSave}>
          {pending && <Spinner />}
          Save
        </Button>
      </div>
    </form>
  )
}

function MemoryRow({
  memory,
  onEdit,
  onDelete,
}: {
  memory: Memory
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <li className="group/memory flex items-start gap-2 rounded-lg bg-muted/50 py-2 pr-1 pl-3 text-sm">
      {/* Clicking the text edits it too; the pencil is the keyboard-reachable way. */}
      <button
        type="button"
        tabIndex={-1}
        className="min-w-0 flex-1 cursor-text text-left leading-relaxed wrap-break-word outline-none"
        onClick={onEdit}
      >
        {memory.text}
      </button>
      <time
        dateTime={memory.createdAt}
        className="shrink-0 pt-0.5 text-xs text-muted-foreground tabular-nums"
      >
        {dateFormat.format(new Date(memory.createdAt))}
      </time>
      <div className="-my-1 flex shrink-0 opacity-100 sm:opacity-0 sm:group-focus-within/memory:opacity-100 sm:group-hover/memory:opacity-100">
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground hover:text-foreground"
          aria-label="Edit memory"
          onClick={onEdit}
        >
          <PencilIcon />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground hover:text-destructive"
          aria-label="Delete memory"
          onClick={onDelete}
        >
          <Trash2Icon />
        </Button>
      </div>
    </li>
  )
}

/** What the agent remembers: it saves these itself, and the user can add, rewrite or delete. */
export function MemoriesSection({ agent }: { agent: Agent }) {
  const { data: memories, isPending, error } = useMemories(agent.id)
  const create = useCreateMemory(agent.id)
  const update = useUpdateMemory(agent.id)
  const remove = useDeleteMemory(agent.id)
  // "new", a memory id, or nothing.
  const [editing, setEditing] = useState<string | null>(null)

  function startEditing(id: string) {
    create.reset()
    update.reset()
    setEditing(id)
  }

  const adding =
    editing === "new" ? (
      <MemoryEditor
        label="New memory"
        pending={create.isPending}
        error={create.error?.message}
        onSave={(text) => create.mutate(text, { onSuccess: () => setEditing(null) })}
        onCancel={() => setEditing(null)}
      />
    ) : null

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-memories">
      <div className="flex items-center justify-between gap-2">
        <h3 id="agent-memories" className="text-sm font-medium">
          Memories
        </h3>
        <div className="flex items-center gap-2">
          {memories && memories.length > 0 ? (
            <span className="text-xs text-muted-foreground tabular-nums">{memories.length}</span>
          ) : null}
          {memories && editing !== "new" && (
            <Button variant="ghost" size="xs" onClick={() => startEditing("new")}>
              <PlusIcon />
              Add memory
            </Button>
          )}
        </div>
      </div>
      {isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-9 w-full rounded-lg" />
          <Skeleton className="h-9 w-4/5 rounded-lg" />
        </div>
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load memories: {error.message}</p>
      ) : memories.length === 0 ? (
        (adding ?? (
          <p className="text-sm text-muted-foreground">
            Nothing saved yet. {agent.name} saves facts and preferences it shouldn't forget, like
            how you like things done. You can add your own too.
          </p>
        ))
      ) : (
        <>
          <ul className="flex flex-col gap-1">
            {memories.map((memory) =>
              editing === memory.id ? (
                <li key={memory.id}>
                  <MemoryEditor
                    initial={memory.text}
                    label="Memory"
                    pending={update.isPending}
                    error={update.error?.message}
                    onSave={(text) =>
                      update.mutate(
                        { memoryId: memory.id, text },
                        { onSuccess: () => setEditing(null) },
                      )
                    }
                    onCancel={() => setEditing(null)}
                  />
                </li>
              ) : (
                <MemoryRow
                  key={memory.id}
                  memory={memory}
                  onEdit={() => startEditing(memory.id)}
                  onDelete={() =>
                    remove.mutate(memory.id, {
                      onSuccess: () => toast.success("Memory deleted"),
                      onError: (e) => toast.error(`Couldn't delete the memory: ${e.message}`),
                    })
                  }
                />
              ),
            )}
          </ul>
          {adding}
        </>
      )}
    </section>
  )
}

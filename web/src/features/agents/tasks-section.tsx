import { cn } from "cn"
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Agent } from "@/lib/api-client"

import { useCreateTask, useDeleteTask, useSetTaskEnabled, useTasks, useUpdateTask } from "./api"
import { describeSchedule, formatDateTime, nextRunLabel, type Task } from "./schedule"
import { TaskForm } from "./task-form"

function TaskItem({
  task,
  now,
  onToggle,
  onEdit,
  onDelete,
}: {
  task: Task
  now: Date
  onToggle: (enabled: boolean) => void
  onEdit: (() => void) | undefined
  onDelete: () => void
}) {
  const [expanded, setExpanded] = useState(false)
  // Deleting asks once, inline (no dialog over the sheet).
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const next = nextRunLabel(task, now)
  const switchId = `task-enabled-${task.id}`

  return (
    <li className="group/task flex items-start gap-3 rounded-lg bg-muted/50 py-2.5 pr-1.5 pl-3 text-sm">
      <div className={cn("flex min-w-0 flex-1 flex-col gap-0.5", !task.enabled && "opacity-70")}>
        <span className="font-medium wrap-break-word">{task.name}</span>
        {task.purpose && (
          <button
            type="button"
            className={cn(
              "text-left wrap-break-word text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
              !expanded && "line-clamp-2",
            )}
            aria-expanded={expanded}
            onClick={() => setExpanded((v) => !v)}
          >
            {task.purpose}
          </button>
        )}
        <span className="text-xs text-muted-foreground">{describeSchedule(task)}</span>
        {task.check && (
          <span className="flex min-w-0 items-baseline gap-1 text-xs text-muted-foreground">
            <span className="shrink-0">Only wakes when this changes:</span>
            <Tooltip>
              <TooltipTrigger
                render={<code />}
                className="min-w-0 truncate font-mono text-foreground/80"
              >
                {task.check}
              </TooltipTrigger>
              <TooltipContent className="font-mono break-all">{task.check}</TooltipContent>
            </Tooltip>
          </span>
        )}
        {next &&
          (task.enabled && task.nextFireAt ? (
            <Tooltip>
              <TooltipTrigger
                render={<span />}
                className="w-fit text-xs text-muted-foreground tabular-nums"
              >
                {next}
              </TooltipTrigger>
              <TooltipContent>{formatDateTime(task.nextFireAt)}</TooltipContent>
            </Tooltip>
          ) : (
            <span className="w-fit text-xs font-medium text-muted-foreground">{next}</span>
          ))}
      </div>
      <div className="flex shrink-0 items-center gap-1 pt-0.5">
        {confirmingDelete ? (
          <>
            <Button variant="ghost" size="xs" onClick={() => setConfirmingDelete(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              size="xs"
              autoFocus
              onClick={() => {
                setConfirmingDelete(false)
                onDelete()
              }}
            >
              Delete
            </Button>
          </>
        ) : (
          <div className="flex opacity-100 sm:opacity-0 sm:group-focus-within/task:opacity-100 sm:group-hover/task:opacity-100">
            {onEdit && (
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Edit task ${task.name}`}
                className="text-muted-foreground hover:text-foreground"
                onClick={onEdit}
              >
                <PencilIcon />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`Delete task ${task.name}`}
              className="text-muted-foreground hover:text-destructive"
              onClick={() => setConfirmingDelete(true)}
            >
              <Trash2Icon />
            </Button>
          </div>
        )}
        <Switch
          id={switchId}
          aria-label={`${task.enabled ? "Pause" : "Resume"} task ${task.name}`}
          checked={task.enabled}
          onCheckedChange={(checked: boolean) => onToggle(checked)}
        />
      </div>
    </li>
  )
}

/** The task list itself (no data fetching), so its states are easy to test. */
export function TaskList({
  agentName,
  tasks,
  now: nowProp,
  onToggle,
  onEdit,
  onDelete,
  editingId = null,
  editor = null,
}: {
  agentName: string
  tasks: Task[]
  now?: Date
  onToggle: (task: Task, enabled: boolean) => void
  onEdit?: (task: Task) => void
  onDelete: (task: Task) => void
  /** The task being edited is shown as `editor` in its place. */
  editingId?: string | null
  editor?: ReactNode
}) {
  // Relative times are computed once per opening of the sheet.
  const [openedAt] = useState(() => new Date())
  const now = nowProp ?? openedAt
  if (tasks.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No tasks yet. Create one, or ask {agentName} to do something on a schedule, like "every
        weekday at 10, give me a quick catch-up".
      </p>
    )
  }
  return (
    <ul className="flex flex-col gap-1" aria-label="Tasks">
      {tasks.map((task) =>
        task.id === editingId ? (
          <li key={task.id}>{editor}</li>
        ) : (
          <TaskItem
            key={task.id}
            task={task}
            now={now}
            onToggle={(enabled) => onToggle(task, enabled)}
            onEdit={onEdit && (() => onEdit(task))}
            onDelete={() => onDelete(task)}
          />
        ),
      )}
    </ul>
  )
}

/** "Next: in 3 hours" or the schedule in words, for the saved toast. */
function savedDescription(task: Task): string {
  return nextRunLabel(task) ?? describeSchedule(task)
}

/** What the agent does on its own schedule: create, edit, pause, resume or delete tasks. */
export function TasksSection({ agent }: { agent: Agent }) {
  const { data: tasks, isPending, error } = useTasks(agent.id)
  const setEnabled = useSetTaskEnabled(agent.id)
  const create = useCreateTask(agent.id)
  const update = useUpdateTask(agent.id)
  const remove = useDeleteTask(agent.id)
  // "new", a task id, or nothing.
  const [editing, setEditing] = useState<string | null>(null)
  const editingTask = tasks?.find((t) => t.id === editing)

  function startEditing(id: string) {
    create.reset()
    update.reset()
    setEditing(id)
  }

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-tasks">
      <div className="flex items-center justify-between gap-2">
        <h3 id="agent-tasks" className="text-sm font-medium">
          Tasks
        </h3>
        <div className="flex items-center gap-2">
          {tasks && tasks.length > 0 ? (
            <span className="text-xs text-muted-foreground tabular-nums">{tasks.length}</span>
          ) : null}
          {tasks && editing !== "new" && (
            <Button variant="ghost" size="xs" onClick={() => startEditing("new")}>
              <PlusIcon />
              New task
            </Button>
          )}
        </div>
      </div>
      {isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-14 w-full rounded-lg" />
        </div>
      ) : error && !tasks ? (
        <p className="text-sm text-destructive">Couldn't load tasks: {error.message}</p>
      ) : (
        <>
          {editing === "new" && (
            <TaskForm
              pending={create.isPending}
              error={create.error?.message}
              onCancel={() => setEditing(null)}
              onSubmit={(body) =>
                create.mutate(body, {
                  onSuccess: (task) => {
                    setEditing(null)
                    toast.success(`Created ${task.name}`, { description: savedDescription(task) })
                  },
                })
              }
            />
          )}
          {(editing !== "new" || (tasks ?? []).length > 0) && (
            <TaskList
              agentName={agent.name}
              tasks={tasks ?? []}
              editingId={editingTask ? editingTask.id : null}
              editor={
                editingTask && (
                  <TaskForm
                    key={editingTask.id}
                    task={editingTask}
                    pending={update.isPending}
                    error={update.error?.message}
                    onCancel={() => setEditing(null)}
                    onSubmit={(body) =>
                      update.mutate(
                        { taskId: editingTask.id, body },
                        {
                          onSuccess: (task) => {
                            setEditing(null)
                            toast.success(`Saved ${task.name}`, {
                              description: savedDescription(task),
                            })
                          },
                        },
                      )
                    }
                  />
                )
              }
              onEdit={(task) => startEditing(task.id)}
              onToggle={(task, enabled) =>
                setEnabled.mutate(
                  { taskId: task.id, enabled },
                  {
                    onSuccess: () =>
                      toast.success(`${enabled ? "Resumed" : "Paused"} ${task.name}`),
                    onError: (e) => toast.error(`Couldn't update ${task.name}: ${e.message}`),
                  },
                )
              }
              onDelete={(task) =>
                remove.mutate(task.id, {
                  onSuccess: () => toast.success(`Deleted ${task.name}`),
                  onError: (e) => toast.error(`Couldn't delete ${task.name}: ${e.message}`),
                })
              }
            />
          )}
        </>
      )}
    </section>
  )
}

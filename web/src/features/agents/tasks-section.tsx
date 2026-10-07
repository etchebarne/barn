import { cn } from "cn"
import { Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Agent } from "@/lib/api-client"

import { useDeleteTask, useSetTaskEnabled, useTasks } from "./api"
import { describeSchedule, formatDateTime, nextRunLabel, type Task } from "./schedule"

function TaskItem({
  task,
  now,
  onToggle,
  onDelete,
}: {
  task: Task
  now: Date
  onToggle: (enabled: boolean) => void
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
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={`Delete task ${task.name}`}
            className="text-muted-foreground opacity-100 hover:text-destructive sm:opacity-0 sm:group-focus-within/task:opacity-100 sm:group-hover/task:opacity-100"
            onClick={() => setConfirmingDelete(true)}
          >
            <Trash2Icon />
          </Button>
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
  onDelete,
}: {
  agentName: string
  tasks: Task[]
  now?: Date
  onToggle: (task: Task, enabled: boolean) => void
  onDelete: (task: Task) => void
}) {
  // Relative times are computed once per opening of the sheet.
  const [openedAt] = useState(() => new Date())
  const now = nowProp ?? openedAt
  if (tasks.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No tasks yet. Ask {agentName} to do something on a schedule, like "every weekday at 10, give
        me a quick catch-up".
      </p>
    )
  }
  return (
    <ul className="flex flex-col gap-1" aria-label="Tasks">
      {tasks.map((task) => (
        <TaskItem
          key={task.id}
          task={task}
          now={now}
          onToggle={(enabled) => onToggle(task, enabled)}
          onDelete={() => onDelete(task)}
        />
      ))}
    </ul>
  )
}

/** What the agent does on its own schedule; pause, resume or delete each task. */
export function TasksSection({ agent }: { agent: Agent }) {
  const { data: tasks, isPending, error } = useTasks(agent.id)
  const setEnabled = useSetTaskEnabled(agent.id)
  const remove = useDeleteTask(agent.id)

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-tasks">
      <div className="flex items-baseline justify-between">
        <h3 id="agent-tasks" className="text-sm font-medium">
          Tasks
        </h3>
        {tasks && tasks.length > 0 ? (
          <span className="text-xs text-muted-foreground tabular-nums">{tasks.length}</span>
        ) : null}
      </div>
      {isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-14 w-full rounded-lg" />
        </div>
      ) : error && !tasks ? (
        <p className="text-sm text-destructive">Couldn't load tasks: {error.message}</p>
      ) : (
        <TaskList
          agentName={agent.name}
          tasks={tasks ?? []}
          onToggle={(task, enabled) =>
            setEnabled.mutate(
              { taskId: task.id, enabled },
              {
                onSuccess: () => toast.success(`${enabled ? "Resumed" : "Paused"} ${task.name}`),
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
    </section>
  )
}

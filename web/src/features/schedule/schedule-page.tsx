import { CalendarClockIcon, ChevronRightIcon, PauseIcon, ZapIcon } from "lucide-react"
import { useEffect, useMemo, useState, type ReactNode } from "react"

import { PageHeader } from "@/components/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import {
  AgentAvatar,
  describeSchedule,
  formatRelative,
  useAgentsById,
  type Task,
} from "@/features/agents"
import type { Agent } from "@/lib/api-client"

import { buildAgenda, formatTime } from "./agenda"
import { useSchedule } from "./api"
import { RunRow } from "./run-row"
import { TaskSheet } from "./task-sheet"

/** The current time, ticking every minute (for "in 5 minutes" labels). */
function useNow(): Date {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 60_000)
    return () => clearInterval(id)
  }, [])
  return now
}

function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-2 border-b py-6 last:border-b-0">
      <div className="flex flex-col gap-0.5">
        <h2 className="text-sm font-medium">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

/** One task in a list: who runs it, what, and a detail on the right. Opens the task's sheet. */
function TaskButton({
  task,
  agent,
  leading,
  trailing,
  onOpen,
}: {
  task: Task
  agent: Agent | undefined
  leading?: ReactNode
  trailing?: ReactNode
  onOpen: () => void
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        className="group/row flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm outline-none select-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        {leading}
        <AgentAvatar id={agent?.id} name={agent?.name ?? "Deleted agent"} size="sm" />
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate font-medium">{task.name}</span>
          <span className="truncate text-xs text-muted-foreground">
            {agent?.name ?? "Deleted agent"}
          </span>
        </span>
        {trailing && <span className="shrink-0 text-xs text-muted-foreground">{trailing}</span>}
        <ChevronRightIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground/60" />
      </button>
    </li>
  )
}

function Empty() {
  return (
    <div className="flex flex-col items-center gap-2 py-16 text-center">
      <CalendarClockIcon aria-hidden="true" className="size-8 text-muted-foreground/60" />
      <p className="text-sm font-medium">Nothing scheduled yet</p>
      <p className="max-w-sm text-sm text-muted-foreground">
        Ask an agent to do something on a schedule, like "every weekday at 9, catch me up on my
        inbox", and it shows up here.
      </p>
    </div>
  )
}

/**
 * Schedule: every agent's tasks in one place. What runs in the coming week (tasks that run many
 * times a day listed once), what runs on app events, what's paused, and the latest runs with
 * how they went. Selecting a task opens its sheet (history, Run now, pause).
 */
export function SchedulePage({
  taskId,
  onTaskChange,
}: {
  taskId: string | undefined
  onTaskChange: (taskId: string | undefined) => void
}) {
  const { data, isPending, error } = useSchedule()
  const agents = useAgentsById()
  const now = useNow()
  const tasks = useMemo(() => new Map((data?.tasks ?? []).map((t) => [t.id, t])), [data])
  const agenda = useMemo(() => buildAgenda(data?.upcoming ?? [], now), [data, now])
  const onEvents = data?.tasks.filter((t) => t.kind === "signal" && t.enabled) ?? []
  const paused =
    data?.tasks.filter((t) => !t.enabled && !(t.kind === "once" && t.lastFiredAt)) ?? []
  const selected = taskId ? tasks.get(taskId) : undefined

  function row(task: Task, props: { leading?: ReactNode; trailing?: ReactNode; key?: string }) {
    return (
      <TaskButton
        key={props.key ?? task.id}
        task={task}
        agent={agents.get(task.agentId)}
        leading={props.leading}
        trailing={props.trailing}
        onOpen={() => onTaskChange(task.id)}
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="truncate text-sm font-medium">Schedule</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-4 py-2 md:px-6">
          {isPending && (
            <div className="flex flex-col gap-3 py-6">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-12" />
              <Skeleton className="h-12" />
            </div>
          )}
          {error && (
            <p className="py-6 text-sm text-destructive">
              Couldn't load the schedule: {error.message}
            </p>
          )}
          {data && data.tasks.length === 0 && <Empty />}
          {data && data.tasks.length > 0 && (
            <>
              <Section title="Coming up" description="The next 7 days.">
                {agenda.frequent.length === 0 && agenda.days.length === 0 && (
                  <p className="text-sm text-muted-foreground">
                    Nothing runs on a clock this week.
                  </p>
                )}
                {agenda.frequent.length > 0 && (
                  <ul className="flex flex-col" aria-label="Runs often">
                    {agenda.frequent.map((f) => {
                      const task = tasks.get(f.taskId)
                      return (
                        task &&
                        row(task, {
                          leading: (
                            <ZapIcon
                              aria-hidden="true"
                              className="size-4 shrink-0 text-muted-foreground"
                            />
                          ),
                          trailing: `${describeSchedule(task)} · next ${formatRelative(f.next.toISOString(), now)}`,
                        })
                      )
                    })}
                  </ul>
                )}
                {agenda.days.map((day) => (
                  <div key={day.key} className="flex flex-col">
                    <h3 className="px-2 pt-2 pb-1 text-xs font-medium text-muted-foreground">
                      {day.label}
                    </h3>
                    <ul className="flex flex-col" aria-label={day.label}>
                      {day.entries.map((entry) => {
                        const task = tasks.get(entry.taskId)
                        return (
                          task &&
                          row(task, {
                            key: `${entry.taskId}-${entry.time.getTime()}`,
                            leading: (
                              <span className="w-16 shrink-0 text-xs text-muted-foreground tabular-nums">
                                {formatTime(entry.time)}
                              </span>
                            ),
                          })
                        )
                      })}
                    </ul>
                  </div>
                ))}
              </Section>
              {onEvents.length > 0 && (
                <Section
                  title="On app events"
                  description="Run when a connected app sends a matching event."
                >
                  <ul className="flex flex-col">
                    {onEvents.map((task) =>
                      row(task, { trailing: describeSchedule(task).replace(/^When /, "") }),
                    )}
                  </ul>
                </Section>
              )}
              {paused.length > 0 && (
                <Section title="Paused">
                  <ul className="flex flex-col">
                    {paused.map((task) =>
                      row(task, {
                        leading: (
                          <PauseIcon
                            aria-hidden="true"
                            className="size-4 shrink-0 text-muted-foreground"
                          />
                        ),
                        trailing: describeSchedule(task),
                      }),
                    )}
                  </ul>
                </Section>
              )}
              <Section
                title="Recent runs"
                description="Checks that found no change are listed in each task."
              >
                {data.recent.length === 0 ? (
                  <p className="text-sm text-muted-foreground">Nothing has run yet.</p>
                ) : (
                  <ul className="flex flex-col divide-y" aria-label="Recent runs">
                    {data.recent.map((run) => {
                      const task = tasks.get(run.taskId)
                      const agent = agents.get(run.agentId)
                      return (
                        <RunRow
                          key={run.id}
                          run={run}
                          now={now}
                          title={
                            <button
                              type="button"
                              className="truncate text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring/50"
                              onClick={() => onTaskChange(run.taskId)}
                            >
                              {task?.name ?? "Deleted task"}
                              <span className="font-normal text-muted-foreground">
                                {" "}
                                · {agent?.name ?? "Deleted agent"}
                              </span>
                            </button>
                          }
                        />
                      )
                    })}
                  </ul>
                )}
              </Section>
            </>
          )}
        </div>
      </div>
      <TaskSheet
        task={selected}
        agent={selected ? agents.get(selected.agentId) : undefined}
        now={now}
        onClose={() => onTaskChange(undefined)}
      />
    </div>
  )
}

import { PlayIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  AgentAvatar,
  describeSchedule,
  nextRunLabel,
  openAgentDetails,
  type Task,
} from "@/features/agents"
import type { Agent } from "@/lib/api-client"

import { useRunTaskNow, useSetTaskEnabled, useTaskRuns } from "./api"
import { RunRow } from "./run-row"

function RunHistory({ task, now }: { task: Task; now: Date }) {
  const { data: runs, isPending, error } = useTaskRuns(task.id)
  if (isPending) {
    return (
      <div className="flex flex-col gap-2">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    )
  }
  if (error) return <p className="text-sm text-destructive">Couldn't load runs: {error.message}</p>
  if (runs.length === 0) {
    return <p className="text-sm text-muted-foreground">It hasn't run yet.</p>
  }
  return (
    <ul className="flex flex-col divide-y" aria-label="Runs">
      {runs.map((run) => (
        <RunRow key={run.id} run={run} now={now} />
      ))}
    </ul>
  )
}

/**
 * A task's details on the Schedule page: what it does and when, a switch to pause it, Run now,
 * and its run history. Editing lives in the agent's settings (opened in place of this sheet,
 * so sheets never stack).
 */
export function TaskSheet({
  task,
  agent,
  now,
  onClose,
}: {
  task: Task | undefined
  agent: Agent | undefined
  now: Date
  onClose: () => void
}) {
  const run = useRunTaskNow()
  const setEnabled = useSetTaskEnabled()
  const next = task ? nextRunLabel(task, now) : null
  const switchId = task ? `schedule-task-enabled-${task.id}` : undefined

  return (
    <Sheet
      open={task !== undefined}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md"
      >
        {task && (
          <>
            <SheetHeader className="flex-row items-center gap-3 border-b pr-12">
              <AgentAvatar id={agent?.id} name={agent?.name ?? "Deleted agent"} size="lg" />
              <div className="flex min-w-0 flex-col gap-0.5">
                <SheetTitle className="truncate">{task.name}</SheetTitle>
                <SheetDescription className="truncate">
                  {agent?.name ?? "Deleted agent"}
                </SheetDescription>
              </div>
            </SheetHeader>
            <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto overscroll-contain p-4">
              <section className="flex flex-col gap-2 text-sm">
                <p className="font-medium">{describeSchedule(task)}</p>
                {next && <p className="text-xs text-muted-foreground">{next}</p>}
                <p className="wrap-break-word whitespace-pre-line text-muted-foreground">
                  {task.purpose}
                </p>
                {task.check && (
                  <p className="flex flex-col gap-1 text-xs text-muted-foreground">
                    Only wakes {agent?.name ?? "the agent"} when this command's output changes:
                    <code className="rounded-md bg-muted px-2 py-1 font-mono break-all text-foreground/80">
                      {task.check}
                    </code>
                  </p>
                )}
              </section>
              <section className="flex flex-wrap items-center gap-2">
                {task.kind !== "signal" && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={run.isPending}
                    onClick={() => run.mutate(task.id)}
                  >
                    {run.isPending ? <Spinner /> : <PlayIcon />}
                    Run now
                  </Button>
                )}
                {agent && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      onClose()
                      openAgentDetails(agent.id)
                    }}
                  >
                    Edit in {agent.name}'s settings
                  </Button>
                )}
                {!(task.kind === "once" && task.lastFiredAt) && (
                  <div className="ml-auto flex items-center gap-2">
                    <Label htmlFor={switchId} className="text-sm font-normal text-muted-foreground">
                      {task.enabled ? "Active" : "Paused"}
                    </Label>
                    <Switch
                      id={switchId}
                      checked={task.enabled}
                      disabled={setEnabled.isPending}
                      onCheckedChange={(enabled) => setEnabled.mutate({ taskId: task.id, enabled })}
                    />
                  </div>
                )}
              </section>
              <section className="flex flex-col gap-1">
                <h3 className="text-sm font-medium">Runs</h3>
                <RunHistory task={task} now={now} />
              </section>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

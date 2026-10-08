import { cn } from "cn"
import { CircleAlertIcon, CircleCheckIcon, CircleDashedIcon, CircleStopIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { formatDateTime, formatRelative } from "@/features/agents"

import { describeOutcome, describeTrigger, formatDuration } from "./agenda"
import type { TaskRun } from "./api"

function OutcomeIcon({ run }: { run: TaskRun }) {
  const { tone } = describeOutcome(run)
  if (tone === "live") return <Spinner className="size-4 text-muted-foreground" />
  const Icon =
    run.outcome === "failed"
      ? CircleAlertIcon
      : run.outcome === "stopped"
        ? CircleStopIcon
        : run.outcome === "acted"
          ? CircleCheckIcon
          : CircleDashedIcon
  return (
    <Icon
      aria-hidden="true"
      className={cn(
        "size-4",
        tone === "bad" && "text-destructive",
        tone === "good" && "text-foreground",
        tone === "muted" && "text-muted-foreground",
      )}
    />
  )
}

/**
 * One run: how it went, when, and what the agent did (or what went wrong). `title` names the
 * task and agent where runs of several tasks are listed together.
 */
export function RunRow({ run, now, title }: { run: TaskRun; now: Date; title?: ReactNode }) {
  const [expanded, setExpanded] = useState(false)
  const { label, tone } = describeOutcome(run)
  const trigger = describeTrigger(run)
  const took =
    run.finishedAt &&
    formatDuration(new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime())
  const detail = run.detail.trim()

  return (
    <li className="flex gap-3 py-2.5 text-sm">
      <span className="flex h-5 shrink-0 items-center">
        <OutcomeIcon run={run} />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        {title && <div className="min-w-0 truncate font-medium">{title}</div>}
        <div className="flex flex-wrap items-baseline gap-x-1.5 text-xs text-muted-foreground">
          <span
            className={cn(
              tone === "bad" && "text-destructive",
              tone === "good" && "text-foreground",
            )}
          >
            {label}
          </span>
          <span aria-hidden="true">·</span>
          <Tooltip>
            <TooltipTrigger render={<span />} className="tabular-nums">
              {formatRelative(run.startedAt, now)}
            </TooltipTrigger>
            <TooltipContent>{formatDateTime(run.startedAt)}</TooltipContent>
          </Tooltip>
          {took && (
            <>
              <span aria-hidden="true">·</span>
              <span className="tabular-nums">took {took}</span>
            </>
          )}
          {trigger && (
            <>
              <span aria-hidden="true">·</span>
              <span>{trigger}</span>
            </>
          )}
        </div>
        {detail && (
          <button
            type="button"
            className={cn(
              "text-left text-xs wrap-break-word whitespace-pre-line text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
              tone === "bad" && "text-destructive/90",
              !expanded && "line-clamp-2",
            )}
            aria-expanded={expanded}
            onClick={() => setExpanded((v) => !v)}
          >
            {detail}
          </button>
        )}
      </div>
    </li>
  )
}

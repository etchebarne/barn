import { Link } from "@tanstack/react-router"
import { cn } from "cn"
import {
  CircleAlertIcon,
  CirclePauseIcon,
  CircleStopIcon,
  PlayIcon,
  RotateCwIcon,
  TriangleAlertIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { openAgentDetails, useRetryAgent } from "@/features/agents"
import type { Agent, Message } from "@/lib/api-client"

import { failureActions, failureTone, type Failure } from "./failure"

function RetryButton({
  agentId,
  disabled,
  label,
}: {
  agentId: string
  disabled: boolean
  label: "Retry" | "Continue"
}) {
  const retry = useRetryAgent(agentId)
  return (
    <>
      <Button
        size="sm"
        variant="outline"
        disabled={disabled || retry.isPending}
        onClick={() => retry.mutate()}
      >
        {retry.isPending ? <Spinner /> : label === "Continue" ? <PlayIcon /> : <RotateCwIcon />}
        {label}
      </Button>
      {retry.error && (
        <span role="alert" className="basis-full text-xs text-destructive">
          Couldn't {label.toLowerCase()}: {retry.error.message}
        </span>
      )}
    </>
  )
}

/**
 * A failed agent turn, inline in the conversation: what went wrong plus the way out (retry,
 * change model, or fix the API key). Warning colors for setup problems the user can fix,
 * destructive colors for errors, and quiet colors for a long turn that paused (Continue).
 */
export function FailureNotice({
  message,
  failure,
  agent,
  isLatest,
}: {
  message: Message
  failure: Failure
  agent: Agent | undefined
  isLatest: boolean
}) {
  const tone = failureTone(failure.reason)
  const actions = failureActions(failure, { isLatest, agent })
  const stopped = failure.reason === "stopped"
  const Icon = stopped
    ? CircleStopIcon
    : tone === "neutral"
      ? CirclePauseIcon
      : tone === "warning"
        ? TriangleAlertIcon
        : CircleAlertIcon
  const hasActions = actions.retry || actions.changeModel || actions.openSettings

  return (
    // Nested radius: container radius = button radius (--radius-md) + padding (p-3).
    <div
      role="note"
      aria-label={stopped ? "Agent stopped" : tone === "neutral" ? "Agent paused" : "Agent error"}
      className={cn(
        "mx-auto flex w-full max-w-lg flex-col gap-2 rounded-[calc(var(--radius-md)+0.75rem)] border p-3 text-sm",
        tone === "neutral" && "bg-muted/50 text-muted-foreground",
        tone === "warning" && "border-warning/40 bg-warning/10 text-warning-foreground",
        tone === "destructive" && "border-destructive/30 bg-destructive/5 text-destructive",
      )}
    >
      <div className="flex gap-2">
        <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <p className="min-w-0 wrap-break-word whitespace-pre-wrap">{message.body}</p>
      </div>
      {hasActions && (
        <div className="flex flex-wrap items-center gap-2 pl-6">
          {actions.retry && (
            <RetryButton
              agentId={failure.agentId}
              disabled={actions.disabled}
              label={tone === "neutral" ? "Continue" : "Retry"}
            />
          )}
          {actions.changeModel && (
            <Button
              size="sm"
              variant="outline"
              disabled={actions.disabled}
              onClick={() => openAgentDetails(failure.agentId, { focus: "model" })}
            >
              Change model
            </Button>
          )}
          {actions.openSettings && (
            <Button
              size="sm"
              variant="outline"
              render={<Link to="/settings" hash="model-provider" />}
              nativeButton={false}
            >
              Open settings
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

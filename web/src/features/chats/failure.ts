import type { Agent, Message, Schemas } from "@/lib/api-client"

export type Failure = Schemas["MessageFailure"]
export type FailureReason = Failure["reason"]

/**
 * A long turn that reached the step limit just paused (neutral); configuration problems the user
 * fixes are warnings; everything else is an error (destructive).
 */
export function failureTone(reason: FailureReason): "neutral" | "warning" | "destructive" {
  if (reason === "too_many_steps") return "neutral"
  return reason === "no_key" || reason === "invalid_key" || reason === "model_blocked"
    ? "warning"
    : "destructive"
}

export type FailureActions = {
  retry: boolean
  changeModel: boolean
  openSettings: boolean
  /** Actions are shown but disabled while the agent is already working. */
  disabled: boolean
}

/**
 * Which actions a failure notice offers. Actions only make sense on the latest message: once
 * anything newer happened, the failed turn is history and its fix may already be applied.
 */
export function failureActions(
  failure: Failure,
  { isLatest, agent }: { isLatest: boolean; agent: Agent | undefined },
): FailureActions {
  return {
    // No retry for an agent that's gone (deleted).
    retry: failure.retryable && isLatest && agent !== undefined,
    changeModel: isLatest && failure.reason === "model_blocked" && agent !== undefined,
    openSettings: isLatest && (failure.reason === "no_key" || failure.reason === "invalid_key"),
    disabled: agent?.activity.state === "working",
  }
}

export function messageFailure(message: Message): Failure | null {
  return message.author.kind === "system" ? (message.failure ?? null) : null
}

import type { Message, Schemas } from "@/lib/api-client"

export type Prompt = Schemas["Prompt"]
export type PromptAnswer = Schemas["PromptAnswer"]

/** "A", "B", … "Z" for option indexes. */
export function optionLetter(index: number): string {
  return String.fromCharCode(65 + index)
}

/** Option index for a letter key ("a"/"A" → 0), or null. */
export function letterIndex(key: string, optionCount: number): number | null {
  if (key.length !== 1) return null
  const index = key.toUpperCase().charCodeAt(0) - 65
  return index >= 0 && index < Math.min(optionCount, 26) ? index : null
}

/** The message with its prompt answered (optimistic update; the server's copy replaces it). */
export function answerMessage(message: Message, answer: PromptAnswer): Message {
  if (!message.prompt) return message
  return { ...message, prompt: { ...message.prompt, status: "answered", answer } }
}

export function dismissMessage(message: Message): Message {
  if (!message.prompt) return message
  return { ...message, prompt: { ...message.prompt, status: "dismissed" } }
}

/** Builds the answer to send, or null if nothing was chosen. */
export function buildAnswer(selected: Iterable<number>, text: string): PromptAnswer | null {
  const indexes = [...new Set(selected)].toSorted((a, b) => a - b)
  const trimmed = text.trim()
  if (indexes.length === 0 && !trimmed) return null
  return {
    ...(indexes.length > 0 ? { selected: indexes } : {}),
    ...(trimmed ? { text: trimmed } : {}),
  }
}

export type PromptRow =
  | { type: "option"; index: number; label: string; chosen: boolean }
  | { type: "text"; text: string }

/**
 * Rows a prompt card shows. Pending: every option. Answered: only what was chosen (the card
 * collapses to the answer), plus a typed answer as its own row. Dismissed: nothing.
 */
export function promptRows(prompt: Prompt): PromptRow[] {
  if (prompt.status === "dismissed") return []
  if (prompt.status === "pending") {
    return prompt.options.map((option, index) => ({
      type: "option",
      index,
      label: option.label,
      chosen: false,
    }))
  }
  const chosen = new Set(prompt.answer?.selected ?? [])
  const rows: PromptRow[] = prompt.options.flatMap((option, index) =>
    chosen.has(index)
      ? [{ type: "option" as const, index, label: option.label, chosen: true }]
      : [],
  )
  const text = prompt.answer?.text?.trim()
  if (text) rows.push({ type: "text", text })
  return rows
}

/** The chat to open after answering, if a chosen option links to one. */
export function chatToOpen(prompt: Prompt, answer: PromptAnswer): string | null {
  for (const index of answer.selected ?? []) {
    const chatId = prompt.options[index]?.opensChatId
    if (chatId) return chatId
  }
  return null
}

/**
 * Keyboard selection applies only to the latest pending choice prompt. Never to approvals: a
 * stray keypress must not approve a gated action.
 */
export function acceptsLetterKeys(prompt: Prompt, isLatest: boolean): boolean {
  return (
    isLatest && prompt.status === "pending" && (prompt.kind === "single" || prompt.kind === "multi")
  )
}

/** Approval options are always ["Approve", "Decline"]. */
export const APPROVE_INDEX = 0
export const DECLINE_INDEX = 1
/** Approves and stops asking for this action from this agent ("standing approval"). */
export const ALWAYS_ALLOW_INDEX = 2

export function approvalAnswer(approve: boolean): PromptAnswer {
  return { selected: [approve ? APPROVE_INDEX : DECLINE_INDEX] }
}

export function alwaysAllowAnswer(): PromptAnswer {
  return { selected: [ALWAYS_ALLOW_INDEX] }
}

/** Newer approvals offer a third option, "Always allow". */
export function offersAlwaysAllow(prompt: Prompt): boolean {
  return prompt.kind === "approval" && prompt.options.length > ALWAYS_ALLOW_INDEX
}

/** How an answered approval came out, or null while pending/dismissed. */
export function approvalOutcome(prompt: Prompt): "approved" | "always" | "declined" | null {
  if (prompt.kind !== "approval" || prompt.status !== "answered") return null
  const selected = prompt.answer?.selected ?? []
  if (selected.includes(ALWAYS_ALLOW_INDEX)) return "always"
  return selected.includes(APPROVE_INDEX) ? "approved" : "declined"
}

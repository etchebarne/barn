import type { Schemas } from "@/lib/api-client"

import { approvalOutcome, type Prompt } from "./logic"

export type ActionPreview = Schemas["ActionPreview"]
export type PreviewField = Schemas["PreviewField"]

/** A JSON value turned into something the card can lay out. */
export type PreviewNode =
  | { kind: "text"; text: string; multiline: boolean; mono: boolean }
  | { kind: "chips"; items: string[] }
  | { kind: "rows"; rows: PreviewRow[] }
  | { kind: "blocks"; items: PreviewRow[][] }
  | { kind: "json"; text: string }
  | { kind: "empty"; text: string }

export type PreviewRow = { key: string; label: string; node: PreviewNode }

/** Below this depth, objects and arrays are shown as compact JSON. */
export const MAX_DEPTH = 3
/** Text longer than this collapses behind "Show more". */
export const COLLAPSE_LINES = 12
const COLLAPSE_CHARS = 900

/** "thread_ts" → "Thread ts", "dueDate" → "Due date", "URL" stays "URL". */
export function humanizeKey(key: string): string {
  if (/^[A-Z0-9]+$/.test(key)) return key
  const words = key
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => (/^[A-Z0-9]{2,}$/.test(w) ? w : w.toLowerCase()))
  const sentence = words.join(" ")
  return sentence.charAt(0).toUpperCase() + sentence.slice(1)
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

function isPrimitive(value: unknown): value is string | number | boolean | null {
  return value === null || ["string", "number", "boolean"].includes(typeof value)
}

/** Booleans read as Yes/No; null as an em dash. */
export function formatPrimitive(value: string | number | boolean | null): string {
  if (value === null) return "—"
  if (typeof value === "boolean") return value ? "Yes" : "No"
  return String(value)
}

/** Ids, hashes and tokens: no spaces and longer than 20 characters. */
export function looksLikeCode(text: string): boolean {
  return text.length > 20 && !/\s/.test(text)
}

function textNode(text: string): PreviewNode {
  return {
    kind: "text",
    text,
    multiline: text.includes("\n"),
    mono: looksLikeCode(text),
  }
}

function objectRows(object: Record<string, unknown>, depth: number): PreviewRow[] {
  return Object.entries(object).map(([key, value]) => ({
    key,
    label: humanizeKey(key),
    node: toPreviewNode(value, depth + 1),
  }))
}

/** Lays out any JSON value: text, chips, nested rows, blocks, or compact JSON past MAX_DEPTH. */
export function toPreviewNode(value: unknown, depth = 0): PreviewNode {
  if (value === undefined || value === null) return { kind: "empty", text: "—" }
  if (typeof value === "string")
    return value === "" ? { kind: "empty", text: "—" } : textNode(value)
  if (typeof value === "number" || typeof value === "boolean") {
    return textNode(formatPrimitive(value))
  }
  if (depth >= MAX_DEPTH) return { kind: "json", text: JSON.stringify(value) }
  if (Array.isArray(value)) {
    if (value.length === 0) return { kind: "empty", text: "None" }
    if (value.every(isPrimitive)) return { kind: "chips", items: value.map(formatPrimitive) }
    if (value.every(isPlainObject)) {
      return { kind: "blocks", items: value.map((item) => objectRows(item, depth)) }
    }
    return { kind: "json", text: JSON.stringify(value) }
  }
  if (isPlainObject(value)) {
    const rows = objectRows(value, depth)
    return rows.length === 0 ? { kind: "empty", text: "—" } : { kind: "rows", rows }
  }
  return { kind: "json", text: JSON.stringify(value) }
}

/** Long text collapses behind "Show more". */
export function isLongText(text: string): boolean {
  return text.split("\n").length > COLLAPSE_LINES || text.length > COLLAPSE_CHARS
}

/** The body as text: strings as they are, anything else as readable JSON. */
export function bodyText(field: PreviewField): string {
  return typeof field.value === "string" ? field.value : JSON.stringify(field.value, null, 2)
}

export type StatusChip = { label: string; tone: "warning" | "done" | "muted" }

/** The chip in the card's header for each state. */
export function previewStatus(prompt: Prompt): StatusChip {
  if (prompt.status === "pending") return { label: "Needs approval", tone: "warning" }
  if (prompt.status === "dismissed") return { label: "Dismissed", tone: "muted" }
  const outcome = approvalOutcome(prompt)
  if (outcome === "always") return { label: "Always allowed", tone: "done" }
  return outcome === "approved"
    ? { label: "Approved", tone: "done" }
    : { label: "Declined", tone: "muted" }
}

/** A card proposing a standing approval itself (the agent asking "may I always…?"). */
export function isStandingApprovalProposal(preview: ActionPreview): boolean {
  return preview.appType === null && preview.title === "Always allow"
}

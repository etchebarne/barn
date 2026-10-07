/**
 * @mentions: highlighting mentioned agents in message text, and the composer's autocomplete.
 * Names are matched case-insensitively and may contain spaces, like the server does.
 */

export type MentionTarget = { id: string; name: string }

export type MentionSegment = string | { mention: MentionTarget; text: string }

const WORD = /[\p{L}\p{N}_]/u

function escapeRegExp(text: string) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

/**
 * Splits `text` into plain strings and `@Name` mentions of `targets`. A mention must start the
 * text or follow a non-word character (so `martin@barn.dev` stays plain), and must not run into
 * more word characters (`@barnyard` isn't `@barn`). Longer names win (`@Ops Bot` over `@Ops`).
 */
export function splitMentions(text: string, targets: MentionTarget[]): MentionSegment[] {
  const named = targets.filter((t) => t.name.trim() !== "")
  if (named.length === 0 || !text.includes("@")) return [text]
  const byName = new Map(named.map((t) => [t.name.toLowerCase(), t]))
  const alternatives = [...byName.keys()].toSorted((a, b) => b.length - a.length).map(escapeRegExp)
  const pattern = new RegExp(`@(${alternatives.join("|")})`, "giu")

  const segments: MentionSegment[] = []
  let last = 0
  for (const match of text.matchAll(pattern)) {
    const start = match.index
    const end = start + match[0].length
    const before = text[start - 1]
    const after = text[end]
    if ((before !== undefined && WORD.test(before)) || (after !== undefined && WORD.test(after))) {
      continue
    }
    const target = byName.get((match[1] ?? "").toLowerCase())
    if (!target) continue
    if (start > last) segments.push(text.slice(last, start))
    segments.push({ mention: target, text: match[0] })
    last = end
  }
  if (last === 0) return [text]
  if (last < text.length) segments.push(text.slice(last))
  return segments
}

/** The agents a message mentions, resolved from ids (unknown ids are skipped). */
export function mentionTargets(
  mentions: string[],
  agents: Map<string, { id: string; name: string }>,
): MentionTarget[] {
  return mentions.flatMap((id) => {
    const agent = agents.get(id)
    return agent ? [{ id, name: agent.name }] : []
  })
}

export type MentionQuery = {
  /** Index of the `@`. */
  start: number
  /** What's been typed after the `@`, up to the caret. */
  query: string
}

const MAX_QUERY = 64

/** The `@query` being typed right before the caret, if any. */
export function activeMentionQuery(text: string, caret: number): MentionQuery | null {
  const before = text.slice(0, caret)
  const at = before.lastIndexOf("@")
  if (at === -1) return null
  const prev = before[at - 1]
  if (prev !== undefined && (WORD.test(prev) || prev === "@")) return null
  const query = before.slice(at + 1)
  if (query.length > MAX_QUERY || query.includes("\n") || /^\s/.test(query)) return null
  return { start: at, query }
}

/**
 * Candidates for a query: names starting with it first, then names with a word starting with
 * it. An empty query lists everyone.
 */
export function filterMentionCandidates<T extends MentionTarget>(
  candidates: T[],
  query: string,
): T[] {
  const q = query.toLowerCase()
  if (!q) return candidates
  const prefix: T[] = []
  const word: T[] = []
  for (const candidate of candidates) {
    const name = candidate.name.toLowerCase()
    if (name.startsWith(q)) prefix.push(candidate)
    else if (name.split(/\s+/).some((part) => part.startsWith(q))) word.push(candidate)
  }
  return [...prefix, ...word]
}

/** Replaces the `@query` (from `start` to `caret`) with `@Name ` and returns the new caret. */
export function insertMention(
  text: string,
  start: number,
  caret: number,
  name: string,
): { text: string; caret: number } {
  const inserted = `@${name} `
  const rest = text.slice(caret).replace(/^ /, "")
  return { text: text.slice(0, start) + inserted + rest, caret: start + inserted.length }
}

import type { Message, Schemas } from "./api-client"

export type Reaction = Schemas["Reaction"]

/** The emojis offered by the quick "React" picker. */
export const QUICK_REACTIONS = ["👍", "❤️", "😂", "🎉", "👀", "🙏"] as const

export function userReacted(reaction: Reaction): boolean {
  return reaction.by.some((author) => author.kind === "user")
}

/**
 * The user's reaction toggle, as the server applies it: removes the user's `emoji` reaction if
 * present (dropping the emoji when nobody's left), otherwise adds it (a new emoji goes last).
 */
export function toggleUserReaction(message: Message, emoji: string): Message {
  const existing = message.reactions.find((r) => r.emoji === emoji)
  let reactions: Reaction[]
  if (!existing) {
    reactions = [...message.reactions, { emoji, by: [{ kind: "user", agentId: null }] }]
  } else if (userReacted(existing)) {
    reactions = message.reactions.flatMap((r) => {
      if (r.emoji !== emoji) return [r]
      const by = r.by.filter((author) => author.kind !== "user")
      return by.length > 0 ? [{ ...r, by }] : []
    })
  } else {
    reactions = message.reactions.map((r) =>
      r.emoji === emoji ? { ...r, by: [...r.by, { kind: "user" as const, agentId: null }] } : r,
    )
  }
  return { ...message, reactions }
}

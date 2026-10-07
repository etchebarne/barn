import type { Agent, Chat, Message } from "@/lib/api-client"

/** One-line plain-text preview of a markdown message body. */
export function plainPreview(body: string, max = 120): string {
  const text = body
    .replace(/```[\s\S]*?```/g, " [code] ")
    .replace(/`([^`]*)`/g, "$1")
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/^\s{0,3}(#{1,6}|>|[-*+]|\d+\.)\s+/gm, "")
    .replace(/[*_~]{1,3}([^*_~]+)[*_~]{1,3}/g, "$1")
    .replace(/\s+/g, " ")
    .trim()
  return text.length > max ? `${text.slice(0, max - 1)}…` : text
}

export const DELETED_AGENT = "Deleted agent"

/**
 * An agent author's display name: its name, "Deleted agent" when the server cleared the id
 * (the agent was deleted), or "Agent" when it isn't loaded (e.g. archived).
 */
export function agentAuthorName(
  agentId: string | null,
  agents: Map<string, Agent>,
): { name: string; deleted: boolean } {
  if (agentId === null) return { name: DELETED_AGENT, deleted: true }
  return { name: agents.get(agentId)?.name ?? "Agent", deleted: false }
}

export function authorName(message: Message, agents: Map<string, Agent>): string {
  if (message.author.kind === "user") return "You"
  if (message.author.kind === "system") return "System"
  return agentAuthorName(message.author.agentId, agents).name
}

/** Sidebar preview: groups prefix the author; DMs only prefix your own messages. */
export function chatPreview(chat: Chat, agents: Map<string, Agent>): string {
  const message = chat.lastMessage
  if (!message) return "No messages yet"
  const text = plainPreview(message.body)
  if (message.author.kind === "user") return `You: ${text}`
  if (chat.kind === "group" && message.author.kind === "agent") {
    return `${authorName(message, agents)}: ${text}`
  }
  return text
}

/** The agent on the other side of a DM. */
export function dmAgent(chat: Chat, agents: Map<string, Agent>): Agent | undefined {
  if (chat.kind !== "dm") return undefined
  const member = chat.members[0]
  return member ? agents.get(member.agentId) : undefined
}

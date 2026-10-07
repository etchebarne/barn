import type { Agent, Chat, Message } from "@/lib/api-client"

let seq = 0

/** ULID-like ids: fixed-width so string order is creation order. */
export function nextId(prefix = "01J"): string {
  seq += 1
  return `${prefix}${String(seq).padStart(8, "0")}`
}

export function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: nextId(),
    chatId: "chat-1",
    author: { kind: "agent", agentId: "agent-1" },
    body: "hello",
    createdAt: "2026-10-06T12:00:00.000Z",
    reactions: [],
    mentions: [],
    attachments: [],
    ...overrides,
  }
}

export function makeChat(overrides: Partial<Chat> = {}): Chat {
  return {
    id: "chat-1",
    kind: "dm",
    name: "barn",
    members: [{ agentId: "agent-1", position: 0 }],
    unreadCount: 0,
    lastMessage: null,
    categoryId: null,
    position: null,
    createdAt: "2026-10-01T00:00:00.000Z",
    ...overrides,
  }
}

export function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    id: "agent-1",
    name: "barn",
    instructions: "",
    model: "kimi-k2.6",
    language: "auto",
    notifications: true,
    trustMode: "ask",
    isAdmin: true,
    activity: { state: "idle", label: null },
    createdAt: "2026-10-01T00:00:00.000Z",
    ...overrides,
  }
}

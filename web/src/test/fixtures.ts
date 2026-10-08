import type { Agent, Chat, Message, Schemas } from "@/lib/api-client"

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
    name: "openbot",
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
    name: "openbot",
    instructions: "",
    personality: "",
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

export function makeUsageTotals(
  overrides: Partial<Schemas["UsageTotals"]> = {},
): Schemas["UsageTotals"] {
  return {
    calls: 0,
    promptTokens: 0,
    cachedTokens: 0,
    cacheWriteTokens: 0,
    completionTokens: 0,
    reasoningTokens: 0,
    ...overrides,
  }
}

/** A usage report over `days` days ending 2026-10-08 (oldest first), empty unless overridden. */
export function makeUsageReport(
  overrides: Partial<Schemas["UsageReport"]> = {},
  days = 7,
): Schemas["UsageReport"] {
  const end = Date.UTC(2026, 9, 8)
  return {
    today: makeUsageTotals(),
    total: makeUsageTotals(),
    days: Array.from({ length: days }, (_, i) => ({
      date: new Date(end - (days - 1 - i) * 86_400_000).toISOString().slice(0, 10),
      ...makeUsageTotals(),
    })),
    byPurpose: [],
    byAgent: [],
    ...overrides,
  }
}

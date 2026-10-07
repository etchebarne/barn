import type { InfiniteData, QueryClient } from "@tanstack/react-query"

import type { Agent, AgentActivity, Chat, Message, MessagePage } from "./api-client"
import { isChatInView } from "./chat-view"
import { queryKeys } from "./query-keys"

/**
 * Pure cache updaters shared by the realtime layer and mutations. Message ids are ULIDs, so
 * string comparison is chronological comparison.
 */

/** Infinite message cache: `pages[0]` is the newest page; later pages are older history. */
export type MessagesData = InfiniteData<MessagePage>

export function compareIds(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

export function hasMessage(data: MessagesData, id: string): boolean {
  return data.pages.some((page) => page.messages.some((m) => m.id === id))
}

/** Chronological, de-duplicated list of every loaded message. */
export function flattenMessages(data: MessagesData | undefined): Message[] {
  if (!data) return []
  const seen = new Set<string>()
  const out: Message[] = []
  for (let i = data.pages.length - 1; i >= 0; i--) {
    for (const message of data.pages[i]?.messages ?? []) {
      if (seen.has(message.id)) continue
      seen.add(message.id)
      out.push(message)
    }
  }
  return out
}

/** Inserts a message into the newest page, keeping order. No-op if it's already cached. */
export function insertMessage(data: MessagesData, message: Message): MessagesData {
  if (hasMessage(data, message.id)) return data
  const [newest, ...rest] = data.pages
  if (!newest) {
    return { ...data, pages: [{ messages: [message], hasMore: false }], pageParams: [undefined] }
  }
  // Messages almost always arrive in order; only sort when they don't.
  const last = newest.messages.at(-1)
  const appended = [...newest.messages, message]
  const messages =
    last && compareIds(last.id, message.id) > 0
      ? appended.toSorted((a, b) => compareIds(a.id, b.id))
      : appended
  return { ...data, pages: [{ ...newest, messages }, ...rest] }
}

const lastActivity = (chat: Chat) => chat.lastMessage?.id ?? ""

function sortChats(chats: Chat[]): Chat[] {
  return chats.toSorted((a, b) => {
    const byActivity = compareIds(lastActivity(b), lastActivity(a))
    return byActivity !== 0 ? byActivity : compareIds(b.createdAt, a.createdAt)
  })
}

/**
 * Updates the chat list for a new message: last message preview, unread count, and
 * most-recently-active ordering. The unread count only grows for messages not written by the
 * user and not already on screen (`inView`).
 * Returns `null` if the chat isn't in the list (caller should refetch).
 */
export function applyMessageToChats(
  chats: Chat[],
  message: Message,
  inView = false,
): Chat[] | null {
  const index = chats.findIndex((chat) => chat.id === message.chatId)
  const chat = chats[index]
  if (!chat) return null
  if (chat.lastMessage && compareIds(chat.lastMessage.id, message.id) >= 0) {
    // Already seen (e.g. the optimistic send and the WS echo both landed).
    return chats
  }
  const unreadCount =
    message.author.kind === "user" ? 0 : inView ? chat.unreadCount : chat.unreadCount + 1
  const next = [...chats]
  next[index] = { ...chat, lastMessage: message, unreadCount }
  return sortChats(next)
}

/**
 * Clears unread for a chat once it's read up to `lastMessageId`. Returns `null` when newer
 * messages exist that we can't count locally (caller should refetch).
 */
export function applyReadToChats(
  chats: Chat[],
  chatId: string,
  lastMessageId: string,
): Chat[] | null {
  const index = chats.findIndex((chat) => chat.id === chatId)
  const chat = chats[index]
  if (!chat) return chats
  if (chat.lastMessage && compareIds(chat.lastMessage.id, lastMessageId) > 0) return null
  if (chat.unreadCount === 0) return chats
  const next = [...chats]
  next[index] = { ...chat, unreadCount: 0 }
  return next
}

export function applyActivityToAgents(
  agents: Agent[],
  agentId: string,
  activity: AgentActivity,
): Agent[] {
  const index = agents.findIndex((agent) => agent.id === agentId)
  const agent = agents[index]
  if (!agent) return agents
  const next = [...agents]
  next[index] = { ...agent, activity }
  return next
}

/** Replaces an agent with its updated version. Returns `null` if it isn't in the list. */
export function replaceAgent(agents: Agent[], agent: Agent): Agent[] | null {
  const index = agents.findIndex((a) => a.id === agent.id)
  if (index === -1) return null
  const next = [...agents]
  next[index] = agent
  return next
}

/** Puts an updated agent (from a mutation or an `agent.updated` event) into the agents list. */
export function updateAgentInCache(queryClient: QueryClient, agent: Agent) {
  let missing = false
  queryClient.setQueryData<Agent[]>(queryKeys.agents, (agents) => {
    if (!agents) return agents
    const next = replaceAgent(agents, agent)
    if (next === null) missing = true
    return next ?? agents
  })
  if (missing) void queryClient.invalidateQueries({ queryKey: queryKeys.agents, exact: true })
}

/** Adds a message to every cache that shows it: the chat's history and the chat list. */
export function addMessageToCache(queryClient: QueryClient, message: Message) {
  queryClient.setQueryData<MessagesData>(queryKeys.messages(message.chatId), (data) =>
    data ? insertMessage(data, message) : data,
  )

  let missing = false
  queryClient.setQueryData<Chat[]>(queryKeys.chats, (chats) => {
    if (!chats) return chats
    const next = applyMessageToChats(chats, message, isChatInView(message.chatId))
    if (next === null) missing = true
    return next ?? chats
  })
  if (missing) void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true })
}

/** Marks a chat read locally (from our own read or a `chat.read` event from another device). */
export function markChatReadInCache(
  queryClient: QueryClient,
  chatId: string,
  lastMessageId: string,
) {
  let stale = false
  queryClient.setQueryData<Chat[]>(queryKeys.chats, (chats) => {
    if (!chats) return chats
    const next = applyReadToChats(chats, chatId, lastMessageId)
    if (next === null) stale = true
    return next ?? chats
  })
  if (stale) void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true })
}

import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { useAgentsById } from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"

import { chatsQueryOptions } from "./api"
import { useDraftStore } from "./draft-store"
import { dmAgent } from "./preview"
import { useReplyStore } from "./reply-store"

/**
 * The DM where new agents and groups get made: the admin agent's (the starter agent), else the
 * first DM. Agents are created by asking an agent, so "New" opens that chat with a draft.
 */
export function creatorChat(chats: Chat[], agents: Map<string, Agent>): Chat | undefined {
  const dms = chats.filter((c) => c.kind === "dm")
  return dms.find((c) => dmAgent(c, agents)?.isAdmin) ?? dms[0]
}

export const NEW_DRAFTS = {
  agent: "Make me a new agent that ",
  group: "Start a group chat with ",
} as const

export type NewKind = keyof typeof NEW_DRAFTS

/** "New agent" / "New group": opens the creator agent's DM with the request started. */
export function useStartNew() {
  const { data: chats = [] } = useQuery(chatsQueryOptions)
  const agents = useAgentsById()
  const navigate = useNavigate()
  const setDraft = useDraftStore((s) => s.setDraft)
  const requestFocus = useReplyStore((s) => s.requestFocus)
  const chat = creatorChat(chats, agents)
  return {
    /** Who gets asked, e.g. "openbot"; undefined when there's no DM yet. */
    creator: chat ? dmAgent(chat, agents) : undefined,
    start: chat
      ? (kind: NewKind) => {
          setDraft(chat.id, NEW_DRAFTS[kind])
          void navigate({ to: "/chats/$chatId", params: { chatId: chat.id } }).then(requestFocus)
        }
      : null,
  }
}

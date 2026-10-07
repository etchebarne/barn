import { useQuery } from "@tanstack/react-query"
import { UsersIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { MessageScrollerProvider } from "@/components/ui/message-scroller"
import { Skeleton } from "@/components/ui/skeleton"
import { activityLabel, AgentAvatar, useAgentsById } from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"

import { ActivityLine } from "./activity-line"
import { chatsQueryOptions, useSendMessage } from "./api"
import { Composer } from "./composer"
import { useDraftStore } from "./draft-store"
import { MessageList } from "./message-list"
import { dmAgent } from "./preview"

function ChatHeader({ chat, members }: { chat: Chat; members: Agent[] }) {
  const agent = members[0]
  const isDm = chat.kind === "dm"
  const status = isDm
    ? agent
      ? (activityLabel(agent) ?? "idle")
      : null
    : `${members.length} ${members.length === 1 ? "agent" : "agents"}`

  return (
    <PageHeader>
      {isDm ? (
        <AgentAvatar name={agent?.name ?? chat.name} />
      ) : (
        <span className="flex size-8 items-center justify-center rounded-full bg-muted">
          <UsersIcon className="size-4 text-muted-foreground" />
        </span>
      )}
      <div className="flex min-w-0 flex-col">
        <h1 className="truncate text-sm leading-tight font-medium">{chat.name}</h1>
        {status && <p className="truncate text-xs text-muted-foreground">{status}</p>}
      </div>
    </PageHeader>
  )
}

function ChatComposer({ chat }: { chat: Chat }) {
  const draft = useDraftStore((s) => s.drafts[chat.id] ?? "")
  const setDraft = useDraftStore((s) => s.setDraft)
  const { send } = useSendMessage(chat.id)
  return (
    <Composer
      value={draft}
      onChange={(text) => setDraft(chat.id, text)}
      onSend={send}
      placeholder={`Message ${chat.name}`}
    />
  )
}

export function ChatView({ chatId }: { chatId: string }) {
  const { data: chats, isPending } = useQuery(chatsQueryOptions)
  const agents = useAgentsById()
  const chat = chats?.find((c) => c.id === chatId)

  if (isPending) {
    return (
      <div className="flex h-svh flex-1 flex-col">
        <PageHeader>
          <Skeleton className="h-5 w-32" />
        </PageHeader>
      </div>
    )
  }

  if (!chat) {
    return (
      <div className="flex h-svh flex-1 flex-col">
        <PageHeader>
          <h1 className="text-sm font-medium">Chat not found</h1>
        </PageHeader>
        <p className="m-auto p-6 text-sm text-muted-foreground">
          This chat doesn't exist or was removed.
        </p>
      </div>
    )
  }

  const members = chat.members
    .toSorted((a, b) => a.position - b.position)
    .map((m) => agents.get(m.agentId))
    .filter((a): a is Agent => a !== undefined)
  const dm = dmAgent(chat, agents)

  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <ChatHeader chat={chat} members={dm ? [dm] : members} />
      <MessageScrollerProvider key={chat.id} autoScroll defaultScrollPosition="last-anchor">
        <MessageList chat={chat} agents={agents} />
      </MessageScrollerProvider>
      <div className="mx-auto w-full max-w-3xl px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <ActivityLine agents={members} />
        <ChatComposer key={chat.id} chat={chat} />
      </div>
    </div>
  )
}

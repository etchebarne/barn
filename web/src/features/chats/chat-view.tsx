import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { cn } from "cn"
import { MonitorIcon, SlidersHorizontalIcon } from "lucide-react"
import type { ComponentProps, ReactNode } from "react"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { MessageScrollerProvider } from "@/components/ui/message-scroller"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import {
  activityLabel,
  AgentAvatar,
  GroupAvatar,
  openAgentDetails,
  useAgentsById,
} from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"

import { chatsQueryOptions, useSendMessage } from "./api"
import { Composer } from "./composer"
import { useDraftStore } from "./draft-store"
import { FileDropZone } from "./file-drop-zone"
import { MessageList } from "./message-list"
import { dmAgent } from "./preview"
import { quoteAuthorName } from "./quote-block"
import { quoteExcerpt, useReplyStore, useReplyTo } from "./reply-store"
import {
  addFiles,
  removeUpload,
  retryUpload,
  takeUploads,
  useUploadNotice,
  useUploads,
} from "./uploads-store"

function HeaderTitle({
  title,
  status,
  live = false,
  inButton = false,
}: {
  title: string
  status: string | null
  /** The status is live work: it shimmers. */
  live?: boolean
  /** Buttons only allow phrasing content, so render spans instead of a heading. */
  inButton?: boolean
}) {
  const Title = inButton ? "span" : "h1"
  return (
    <span className="flex min-w-0 flex-col text-left">
      <Title className="truncate text-sm leading-5 font-semibold">{title}</Title>
      {status && (
        <span
          className={cn(
            "truncate text-xs leading-4 font-normal text-muted-foreground",
            live && "shimmer",
          )}
        >
          {status}
        </span>
      )}
    </span>
  )
}

function HeaderIconLink({
  label,
  children,
  ...props
}: { label: string; children: ReactNode } & ComponentProps<"button">) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            aria-label={label}
            className="text-muted-foreground hover:text-foreground"
            {...props}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function ChatHeader({ chat, members }: { chat: Chat; members: Agent[] }) {
  const agent = members[0]
  const navigate = useNavigate()

  if (chat.kind === "dm") {
    const working = agent ? activityLabel(agent) : null
    // Idle: which model it runs on. Working: what it's doing.
    const status = agent ? (working ?? agent.model) : null
    return (
      <PageHeader
        actions={
          agent ? (
            <>
              <HeaderIconLink
                label="Computer"
                onClick={() =>
                  void navigate({ to: "/agents/$agentId/computer", params: { agentId: agent.id } })
                }
              >
                <MonitorIcon />
              </HeaderIconLink>
              <HeaderIconLink label="Agent settings" onClick={() => openAgentDetails(agent.id)}>
                <SlidersHorizontalIcon />
              </HeaderIconLink>
            </>
          ) : undefined
        }
      >
        {agent ? (
          <>
            <h1 className="sr-only">{chat.name}</h1>
            <Button
              variant="ghost"
              // shrink: buttons don't by default, and a long status would run under the actions.
              className="-ml-1.5 h-11 max-w-full min-w-0 shrink justify-start gap-2.5 overflow-hidden rounded-xl px-1.5 hover:bg-accent"
              aria-haspopup="dialog"
              aria-label={`${agent.name}: agent details`}
              onClick={() => openAgentDetails(agent.id)}
            >
              <AgentAvatar
                id={agent.id}
                name={agent.name}
                active={agent.activity.state === "working"}
              />
              <HeaderTitle title={chat.name} status={status} live={working !== null} inButton />
            </Button>
          </>
        ) : (
          <>
            <AgentAvatar name={chat.name} />
            <HeaderTitle title={chat.name} status={null} />
          </>
        )}
      </PageHeader>
    )
  }

  const working = members.filter((m) => m.activity.state === "working")
  return (
    <PageHeader>
      <GroupAvatar memberIds={members.map((m) => m.id)} />
      <HeaderTitle
        title={chat.name}
        status={
          working.length > 0
            ? `${joinNames(working.map((m) => m.name))} ${working.length === 1 ? "is" : "are"} working…`
            : joinNames(members.map((m) => m.name)) || null
        }
        live={working.length > 0}
      />
    </PageHeader>
  )
}

/** "Alpha", "Alpha and Beta", "Alpha, Beta and openbot". */
export function joinNames(names: string[]): string {
  if (names.length <= 1) return names[0] ?? ""
  return `${names.slice(0, -1).join(", ")} and ${names.at(-1)}`
}

export function ChatComposer({ chat, members }: { chat: Chat; members: Agent[] }) {
  const draft = useDraftStore((s) => s.drafts[chat.id] ?? "")
  const setDraft = useDraftStore((s) => s.setDraft)
  const { send } = useSendMessage(chat.id)
  const uploads = useUploads(chat.id)
  const uploadNotice = useUploadNotice(chat.id)
  const replyTo = useReplyTo(chat.id)
  const cancelReply = useReplyStore((s) => s.cancelReply)
  const focusRequest = useReplyStore((s) => s.focusRequest)
  const agents = useAgentsById()
  return (
    <Composer
      value={draft}
      onChange={(text) => setDraft(chat.id, text)}
      onSend={(text) => {
        // Finished uploads go with the message, in the order they were added.
        const attached = takeUploads(chat.id).flatMap((u) =>
          u.attachment ? [{ attachment: u.attachment, previewUrl: u.previewUrl }] : [],
        )
        send(text, attached, replyTo)
        cancelReply(chat.id)
      }}
      replyTo={
        replyTo
          ? {
              name: quoteAuthorName(replyTo.author, agents),
              excerpt: quoteExcerpt(replyTo, 120),
            }
          : null
      }
      onCancelReply={() => cancelReply(chat.id)}
      focusRequest={focusRequest}
      uploads={uploads}
      uploadNotice={uploadNotice}
      onAddFiles={(files) => addFiles(chat.id, files)}
      onRemoveUpload={(id) => removeUpload(chat.id, id)}
      onRetryUpload={(id) => retryUpload(chat.id, id)}
      placeholder={`Message ${chat.name}`}
      mentionCandidates={
        chat.kind === "group" ? members.map((m) => ({ id: m.id, name: m.name })) : undefined
      }
    />
  )
}

export function ChatView({ chatId }: { chatId: string }) {
  const { data: chats, isPending } = useQuery(chatsQueryOptions)
  const agents = useAgentsById()
  const chat = chats?.find((c) => c.id === chatId)

  if (isPending) {
    return (
      <div className="flex h-full min-h-0 flex-1 flex-col">
        <PageHeader>
          <Skeleton className="h-5 w-32" />
        </PageHeader>
      </div>
    )
  }

  if (!chat) {
    return (
      <div className="flex h-full min-h-0 flex-1 flex-col">
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
    <FileDropZone onFiles={(files) => addFiles(chat.id, files)}>
      <ChatHeader chat={chat} members={dm ? [dm] : members} />
      <MessageScrollerProvider key={chat.id} autoScroll defaultScrollPosition="last-anchor">
        <MessageList chat={chat} agents={agents} members={members} />
      </MessageScrollerProvider>
      <div className="mx-auto w-full max-w-3xl px-5 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <ChatComposer key={chat.id} chat={chat} members={members} />
      </div>
    </FileDropZone>
  )
}

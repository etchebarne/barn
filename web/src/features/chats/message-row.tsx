import { Link } from "@tanstack/react-router"
import { cn } from "cn"
import type { ReactNode } from "react"

import { CopyButton } from "@/components/copy-button"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { Message, MessageAvatar, MessageContent, MessageHeader } from "@/components/ui/message"
import { AgentAvatar } from "@/features/agents"
import { PromptCard } from "@/features/prompts"
import type { Agent, Schemas } from "@/lib/api-client"

import { messageFailure } from "./failure"
import { FailureNotice } from "./failure-notice"
import type { Row } from "./grouping"
import { formatDay, formatTime } from "./grouping"
import { Markdown } from "./markdown"
import type { PendingMessage } from "./pending-store"

/** Per-message actions, revealed on hover or keyboard focus (always shown on touch screens). */
function MessageActions({ side, children }: { side: "start" | "end"; children: ReactNode }) {
  return (
    <div
      className={cn(
        "absolute bottom-0 flex items-center gap-1 text-xs whitespace-nowrap text-muted-foreground",
        "opacity-0 group-focus-within/message:opacity-100 group-hover/message:opacity-100 [@media(hover:none)]:opacity-100",
        side === "start" ? "left-full pl-1" : "right-full flex-row-reverse pr-1",
      )}
    >
      {children}
    </div>
  )
}

/** "Created scout": a centered note linking to what happened (e.g. the new agent's DM). */
function EventMarker({
  event,
  agents,
  fallback,
}: {
  event: Schemas["MessageEvent"]
  agents: Map<string, Agent>
  fallback: string
}) {
  const agent = agents.get(event.agentId)
  const label = agent ? `Created ${agent.name}` : fallback || "Created a new agent"
  const content = (
    <>
      <AgentAvatar name={agent?.name ?? "?"} size="sm" />
      <MarkerContent className="font-medium">{label}</MarkerContent>
    </>
  )
  return (
    <Marker className="justify-center py-1 text-xs select-none">
      {event.chatId ? (
        <Link
          to="/chats/$chatId"
          params={{ chatId: event.chatId }}
          className="inline-flex items-center gap-2 rounded-full border bg-background py-1 pr-3 pl-1 no-underline! hover:bg-muted"
        >
          {content}
        </Link>
      ) : (
        <span className="inline-flex items-center gap-2 rounded-full border py-1 pr-3 pl-1">
          {content}
        </span>
      )}
    </Marker>
  )
}

export function DaySeparator({ iso }: { iso: string }) {
  return (
    <Marker variant="separator" className="py-2 text-xs select-none">
      <MarkerContent>{formatDay(iso)}</MarkerContent>
    </Marker>
  )
}

export function MessageRow({
  row,
  agents,
  showAuthorName,
  isLatest,
}: {
  row: Row
  agents: Map<string, Agent>
  showAuthorName: boolean
  /** Last message in the chat (nothing newer, not even a pending send). */
  isLatest: boolean
}) {
  const { message, startsRun, endsRun } = row
  const agent = message.author.agentId ? agents.get(message.author.agentId) : undefined

  const failure = messageFailure(message)
  if (failure) {
    return (
      <FailureNotice
        message={message}
        failure={failure}
        agent={agents.get(failure.agentId)}
        isLatest={isLatest}
      />
    )
  }

  if (message.author.kind === "system" && message.event) {
    return <EventMarker event={message.event} agents={agents} fallback={message.body} />
  }

  if (message.author.kind === "system") {
    return (
      <Marker className="justify-center py-1 text-center text-xs">
        <MarkerContent>{message.body}</MarkerContent>
      </Marker>
    )
  }

  const time = (
    <time dateTime={message.createdAt} className="hidden tabular-nums sm:inline">
      {formatTime(message.createdAt)}
    </time>
  )

  if (message.author.kind === "user") {
    return (
      <Message align="end">
        <MessageContent>
          <Bubble variant="default" align="end">
            <BubbleContent className="whitespace-pre-wrap">{message.body}</BubbleContent>
            <MessageActions side="end">
              <CopyButton text={message.body} />
              {time}
            </MessageActions>
          </Bubble>
        </MessageContent>
      </Message>
    )
  }

  const name = agent?.name ?? "Agent"
  return (
    <Message align="start">
      <MessageAvatar className="size-8 bg-transparent">
        {endsRun ? <AgentAvatar name={name} /> : null}
      </MessageAvatar>
      <MessageContent>
        {showAuthorName && startsRun && <MessageHeader>{name}</MessageHeader>}
        {message.prompt ? (
          <Bubble variant="muted" className="w-full max-w-[min(85%,26rem)]">
            {/* p-2: the prompt card's row radii are derived from this padding. */}
            <BubbleContent className="w-full p-2">
              <PromptCard message={message} prompt={message.prompt} isLatest={isLatest} />
            </BubbleContent>
          </Bubble>
        ) : (
          <Bubble variant="muted">
            <BubbleContent>
              <Markdown>{message.body}</Markdown>
            </BubbleContent>
            <MessageActions side="start">
              <CopyButton text={message.body} />
              {time}
            </MessageActions>
          </Bubble>
        )}
      </MessageContent>
    </Message>
  )
}

export function PendingRow({
  pending,
  onRetry,
  onDiscard,
}: {
  pending: PendingMessage
  onRetry: () => void
  onDiscard: () => void
}) {
  const failed = pending.status === "failed"
  return (
    <Message align="end">
      <MessageContent>
        <Bubble variant={failed ? "destructive" : "default"} align="end">
          <BubbleContent className={cn("whitespace-pre-wrap", !failed && "opacity-70")}>
            {pending.body}
          </BubbleContent>
        </Bubble>
        {failed ? (
          <div
            className="flex items-center justify-end gap-1 text-xs text-destructive"
            role="alert"
          >
            Not sent.
            <Button variant="ghost" size="xs" onClick={onRetry}>
              Retry
            </Button>
            <Button variant="ghost" size="xs" className="text-muted-foreground" onClick={onDiscard}>
              Discard
            </Button>
          </div>
        ) : (
          <span className="text-right text-xs text-muted-foreground">Sending…</span>
        )}
      </MessageContent>
    </Message>
  )
}

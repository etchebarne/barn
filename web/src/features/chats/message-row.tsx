import { Link } from "@tanstack/react-router"
import { cn } from "cn"
import { UserRoundXIcon } from "lucide-react"
import { useState, type MouseEvent, type ReactNode } from "react"

import { CopyButton } from "@/components/copy-button"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import { AgentAvatar } from "@/features/agents"
import { PromptCard } from "@/features/prompts"
import type { Agent, Schemas } from "@/lib/api-client"
import { mentionTargets } from "@/lib/mentions"

import { messageFailure } from "./failure"
import { FailureNotice } from "./failure-notice"
import type { Row } from "./grouping"
import { formatDay, formatTime } from "./grouping"
import { Markdown } from "./markdown"
import { MentionText } from "./mention-text"
import { MessageAttachments, type ShownAttachment } from "./message-attachments"
import { MessageReactions, ReactButton } from "./message-reactions"
import type { PendingMessage } from "./pending-store"
import { agentAuthorName } from "./preview"
import { QuoteBlock, ReplyButton } from "./quote-block"

/**
 * Per-message actions (copy, react, reply, time). With a mouse they're revealed on hover or
 * keyboard focus: beside a bubble, bottom-aligned, or (`inline`) in the footer row of a flat
 * reply. On touch screens they're hidden until the message is tapped, then shown as a row under
 * the message (clear of the reactions pill and the screen edge).
 */
function MessageActions({
  side,
  reacted,
  inline = false,
  children,
}: {
  side: "start" | "end"
  reacted: boolean
  inline?: boolean
  children: ReactNode
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-0.5 text-xs whitespace-nowrap text-muted-foreground select-none",
        // Pointer devices: instant reveal (hover feedback is never animated).
        "opacity-0 group-focus-within/message:opacity-100 group-hover/message:opacity-100 has-data-popup-open:opacity-100",
        !inline && "absolute bottom-0",
        !inline && (side === "start" ? "left-full pl-1.5" : "right-full flex-row-reverse pr-1.5"),
        // Touch: in flow under the message, only for the tapped one.
        "[@media(hover:none)]:static [@media(hover:none)]:hidden [@media(hover:none)]:pr-0 [@media(hover:none)]:pl-0 [@media(hover:none)]:opacity-100 [@media(hover:none)]:group-data-[revealed=true]/message:flex",
        // Below the reactions pill when there is one (it hangs off the bubble's bottom edge).
        !inline && (reacted ? "[@media(hover:none)]:pt-6" : "[@media(hover:none)]:pt-1"),
      )}
    >
      {children}
    </div>
  )
}

/** On touch screens, tapping a message (not its links or buttons) shows or hides its actions. */
function useTapToReveal() {
  const [revealed, setRevealed] = useState(false)
  return {
    revealed,
    "data-revealed": revealed,
    onClick: (event: MouseEvent<HTMLElement>) => {
      if (!window.matchMedia?.("(hover: none)").matches) return
      const target = event.target
      if (
        target instanceof Element &&
        target.closest("a, button, input, textarea, [role=button]")
      ) {
        return
      }
      setRevealed((value) => !value)
    },
  }
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
  // Deleted agents aren't in the agents list; their marker stays but no longer links.
  const agent = agents.get(event.agentId)
  const label = agent ? `Created ${agent.name}` : `${fallback || "Created an agent"} (deleted)`
  const content = (
    <>
      <AgentAvatar id={agent?.id ?? event.agentId} name={agent?.name ?? "?"} size="sm" />
      <MarkerContent className="font-medium">{label}</MarkerContent>
    </>
  )
  return (
    <Marker className="justify-center py-1 text-xs select-none">
      {event.chatId && agent ? (
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

/**
 * Replies with code, tables or headings read better as a document than squeezed into a bubble:
 * they render flat, at the column's full width.
 */
export function isRichBody(body: string): boolean {
  return /```|^\s*\|.+\|\s*$|^#{1,6}\s/m.test(body)
}

/** A run's header in group chats: avatar in the gutter, then name and time. */
function AuthorHeader({
  agent,
  name,
  deleted,
  at,
}: {
  agent: Agent | undefined
  name: string
  deleted: boolean
  at: string
}) {
  return (
    <div className="-ml-8 flex h-6 items-center gap-2 select-none">
      {deleted ? (
        <span
          aria-hidden="true"
          className="flex size-6 items-center justify-center rounded-full border border-dashed text-muted-foreground"
        >
          <UserRoundXIcon className="size-3.5" />
        </span>
      ) : (
        <AgentAvatar id={agent?.id} name={name} size="sm" />
      )}
      <MessageHeader
        className={cn(
          "px-0 text-[13px] text-foreground",
          deleted && "font-normal text-muted-foreground",
        )}
      >
        {name}
      </MessageHeader>
      <time dateTime={at} className="text-xs text-muted-foreground tabular-nums">
        {formatTime(at)}
      </time>
    </div>
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
  /** Group chats: name the author (and show their avatar) at the start of each run. */
  showAuthorName: boolean
  /** Last message in the chat (nothing newer, not even a pending send). */
  isLatest: boolean
}) {
  const { message, startsRun } = row
  const { revealed, ...reveal } = useTapToReveal()
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

  // Rows clip what overflows them (content-visibility), so reacted bubbles reserve room for the
  // reactions badge hanging below them: 3/4 of its height plus its ring.
  const reacted = message.reactions.length > 0
  // A message can be only attachments: then there's no text bubble.
  const hasText = message.body.trim() !== ""
  const mentions = mentionTargets(message.mentions, agents)
  const time = (
    <time dateTime={message.createdAt} className="hidden px-1 tabular-nums sm:inline">
      {formatTime(message.createdAt)}
    </time>
  )

  if (message.author.kind === "user") {
    return (
      <Message align="end" {...reveal}>
        <MessageContent>
          {hasText && <MessageAttachments attachments={message.attachments} align="end" />}
          <Bubble
            variant={hasText ? "secondary" : "ghost"}
            align="end"
            className={cn("max-w-[min(85%,30rem)]", reacted && !revealed && "mb-6")}
          >
            {hasText ? (
              <BubbleContent className="whitespace-pre-wrap">
                {message.replyTo && (
                  <QuoteBlock quote={message.replyTo} agents={agents} align="end" inBubble />
                )}
                <MentionText text={message.body} mentions={mentions} />
              </BubbleContent>
            ) : (
              <>
                {message.replyTo && (
                  <QuoteBlock quote={message.replyTo} agents={agents} align="end" />
                )}
                <MessageAttachments attachments={message.attachments} align="end" />
              </>
            )}
            <MessageReactions message={message} agents={agents} align="start" lifted={revealed} />
            <MessageActions side="end" reacted={reacted}>
              <ReplyButton message={message} />
              <ReactButton message={message} />
              {hasText && <CopyButton text={message.body} />}
              {time}
            </MessageActions>
          </Bubble>
        </MessageContent>
      </Message>
    )
  }

  const { name, deleted } = agentAuthorName(message.author.agentId, agents)
  const header = (showAuthorName || deleted) && startsRun
  const flat = hasText && isRichBody(message.body)
  const actions = (inline: boolean) => (
    <MessageActions side="start" reacted={reacted} inline={inline}>
      {hasText && <CopyButton text={message.body} />}
      <ReactButton message={message} />
      <ReplyButton message={message} />
      {time}
    </MessageActions>
  )

  return (
    // Group chats keep a gutter for the run's avatar, so every message in the run lines up.
    <Message align="start" className={cn((showAuthorName || deleted) && "pl-8")} {...reveal}>
      <MessageContent className="gap-1.5">
        {header && (
          <AuthorHeader agent={agent} name={name} deleted={deleted} at={message.createdAt} />
        )}
        {message.prompt ? (
          <div
            className={cn(
              "relative w-full",
              // Action previews and connect cards carry content, so they get more room.
              message.prompt.preview || message.prompt.connection
                ? "max-w-[min(100%,36rem)]"
                : "max-w-[min(100%,28rem)]",
              reacted && "mb-6",
            )}
          >
            {message.replyTo && (
              <QuoteBlock
                quote={message.replyTo}
                agents={agents}
                align="start"
                className="mb-1.5"
              />
            )}
            <PromptCard message={message} prompt={message.prompt} isLatest={isLatest} />
            <MessageReactions message={message} agents={agents} align="end" />
          </div>
        ) : flat ? (
          // Rich replies: a document, not a bubble. Reactions and actions share a footer row.
          <div className="w-full min-w-0">
            {message.replyTo && (
              <QuoteBlock quote={message.replyTo} agents={agents} align="start" className="mb-2" />
            )}
            <MessageAttachments attachments={message.attachments} align="start" />
            <div className="text-sm leading-relaxed">
              <Markdown mentions={mentions}>{message.body}</Markdown>
            </div>
            <div className="mt-1 flex min-h-7 items-center gap-2">
              <MessageReactions message={message} agents={agents} align="start" inline />
              {actions(true)}
            </div>
          </div>
        ) : (
          <>
            {hasText && <MessageAttachments attachments={message.attachments} align="start" />}
            <Bubble
              variant={hasText ? "muted" : "ghost"}
              className={cn("max-w-[min(85%,42rem)]", reacted && !revealed && "mb-6")}
            >
              {hasText ? (
                <BubbleContent>
                  {message.replyTo && (
                    <QuoteBlock quote={message.replyTo} agents={agents} align="start" inBubble />
                  )}
                  <Markdown mentions={mentions}>{message.body}</Markdown>
                </BubbleContent>
              ) : (
                <>
                  {message.replyTo && (
                    <QuoteBlock quote={message.replyTo} agents={agents} align="start" />
                  )}
                  <MessageAttachments attachments={message.attachments} align="start" />
                </>
              )}
              <MessageReactions message={message} agents={agents} align="end" lifted={revealed} />
              {actions(false)}
            </Bubble>
          </>
        )}
      </MessageContent>
    </Message>
  )
}

export function PendingRow({
  pending,
  agents,
  onRetry,
  onDiscard,
}: {
  pending: PendingMessage
  agents: Map<string, Agent>
  onRetry: () => void
  onDiscard: () => void
}) {
  const failed = pending.status === "failed"
  const attachments: ShownAttachment[] = (pending.attachments ?? []).map((a) =>
    Object.assign({}, a.attachment, { previewUrl: a.previewUrl }),
  )
  return (
    <Message align="end">
      <MessageContent>
        {pending.replyTo && pending.body.trim() === "" && (
          <QuoteBlock quote={pending.replyTo} agents={agents} align="end" />
        )}
        {attachments.length > 0 && (
          <div className={cn(!failed && "opacity-70")}>
            <MessageAttachments attachments={attachments} align="end" />
          </div>
        )}
        {pending.body.trim() !== "" && (
          <Bubble
            variant={failed ? "destructive" : "secondary"}
            align="end"
            className="max-w-[min(85%,30rem)]"
          >
            <BubbleContent className={cn("whitespace-pre-wrap", !failed && "opacity-70")}>
              {pending.replyTo && (
                <QuoteBlock quote={pending.replyTo} agents={agents} align="end" inBubble />
              )}
              {pending.body}
            </BubbleContent>
          </Bubble>
        )}
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
          // The faded bubble already says it's on its way; no label flickering under each send.
          <span className="sr-only" role="status">
            Sending…
          </span>
        )}
      </MessageContent>
    </Message>
  )
}

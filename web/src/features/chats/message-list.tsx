import { useInfiniteQuery } from "@tanstack/react-query"
import { cn } from "cn"
import { ArrowDownIcon } from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState, type UIEvent } from "react"

import { Button } from "@/components/ui/button"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerViewport,
  useMessageScroller,
} from "@/components/ui/message-scroller"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import type { Agent, Chat } from "@/lib/api-client"
import { flattenMessages } from "@/lib/chat-cache"

import { ActivityIndicator } from "./activity-line"
import { messagesQueryOptions, useSendMessage } from "./api"
import { toRows } from "./grouping"
import { DaySeparator, MessageRow, PendingRow } from "./message-row"
import { usePaletteStore } from "./palette-store"
import { reconcilePending, usePendingMessages } from "./pending-store"
import { JumpToMessageContext } from "./quote-block"
import { useMarkReadWhenSeen } from "./read-tracker"

/** Start loading older history when this close to the top. */
const LOAD_OLDER_THRESHOLD_PX = 400

function ListSkeleton() {
  return (
    <div
      className="mx-auto flex w-full max-w-3xl flex-1 flex-col justify-end gap-4 px-5 py-6"
      aria-hidden="true"
    >
      <Skeleton className="h-10 w-1/2 self-end rounded-[20px]" />
      <Skeleton className="h-16 w-3/4 rounded-[20px]" />
      <Skeleton className="h-10 w-2/5 self-end rounded-[20px]" />
      <Skeleton className="h-24 w-2/3 rounded-[20px]" />
    </div>
  )
}

/** Where the user's unread messages start, as of opening the chat. */
function UnreadDivider() {
  return (
    <div
      className="my-3 flex items-center gap-3 text-[11px] font-semibold tracking-wide text-brand-foreground uppercase select-none"
      role="separator"
      aria-label="New messages"
    >
      <span className="h-px flex-1 bg-brand/40" />
      New
      <span className="h-px flex-1 bg-brand/40" />
    </div>
  )
}

/**
 * The first unread message: walking back from the newest, past `unread` messages the user
 * didn't write. null when nothing is unread (or it's further back than what's loaded).
 */
export function firstUnreadId(
  messages: { id: string; author: { kind: string } }[],
  unread: number,
) {
  if (unread <= 0) return null
  let left = unread
  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i]
    if (message.author.kind === "user") continue
    left--
    if (left === 0) return message.id
  }
  return null
}

/** Must be rendered inside a MessageScrollerProvider keyed by chat id. */
export function MessageList({
  chat,
  agents,
  members,
}: {
  chat: Chat
  agents: Map<string, Agent>
  /** The chat's agents, for what they're doing right now. */
  members: Agent[]
}) {
  const query = useInfiniteQuery(messagesQueryOptions(chat.id))
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = query
  const messages = useMemo(() => flattenMessages(query.data), [query.data])
  const rows = useMemo(() => toRows(messages), [messages])
  const pendingAll = usePendingMessages(chat.id)
  const { visible: pending, rowKeys } = useMemo(
    () => reconcilePending(pendingAll, messages),
    [pendingAll, messages],
  )
  // Stable row identity from pending bubble to delivered message (see reconcilePending).
  const rowKey = (id: string) => rowKeys.get(id) ?? id
  // Only the newest user turn is a scroll anchor; older ones never need re-anchoring.
  const lastUserId = messages.findLast((m) => m.author.kind === "user")?.id
  const { retry, discard } = useSendMessage(chat.id)

  // The "New" divider stays where the unread messages started when the chat was opened, even
  // after they're marked read.
  const [unreadFrom, setUnreadFrom] = useState<string | null | undefined>(undefined)
  if (unreadFrom === undefined && !query.isPending) {
    setUnreadFrom(firstUnreadId(messages, chat.unreadCount))
  }

  // Jumping to a quoted message: load older pages until it's there, scroll to it, flash it.
  const { scrollToMessage } = useMessageScroller()
  const [flashId, setFlashId] = useState<string | null>(null)
  const flashTimer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(flashTimer.current), [])

  const reveal = useCallback(
    (messageId: string) => {
      // Wait for freshly loaded history to be in the DOM before scrolling to it: the next frame,
      // or a short timeout when frames aren't running (e.g. a backgrounded window).
      let done = false
      const run = (fn: () => void) => () => {
        if (done) return
        done = true
        fn()
      }
      const go = run(() => {
        const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false
        scrollToMessage(rowKeys.get(messageId) ?? messageId, {
          align: "center",
          behavior: reduced ? "auto" : "smooth",
        })
        setFlashId(messageId)
        clearTimeout(flashTimer.current)
        flashTimer.current = setTimeout(() => setFlashId(null), 1200)
      })
      requestAnimationFrame(go)
      setTimeout(go, 50)
    },
    [rowKeys, scrollToMessage],
  )

  const jumpTo = useCallback(
    async (messageId: string) => {
      // Older pages load one at a time until the message shows up (at most 20 pages, about
      // 1000 messages; past that, nothing happens).
      async function find(
        data: typeof query.data,
        more: boolean,
        budget: number,
      ): Promise<boolean> {
        if (flattenMessages(data).some((m) => m.id === messageId)) return true
        if (!more || budget === 0) return false
        const result = await fetchNextPage()
        return find(result.data, result.hasNextPage, budget - 1)
      }
      if (await find(query.data, hasNextPage, 20)) reveal(messageId)
    },
    [query.data, hasNextPage, fetchNextPage, reveal],
  )

  // A search result in this chat: jump to it once the newest page is in.
  const jump = usePaletteStore((s) => s.jump)
  useEffect(() => {
    if (!jump || jump.chatId !== chat.id || query.isPending) return
    usePaletteStore.getState().clearJump()
    void jumpTo(jump.messageId)
  }, [jump, chat.id, query.isPending, jumpTo])

  const latest = messages.at(-1)
  const lastPending = pending.at(-1)
  useMarkReadWhenSeen({
    chatId: chat.id,
    loaded: !query.isPending,
    latest,
    latestRowId: lastPending ? lastPending.clientId : latest && rowKey(latest.id),
    unread: chat.unreadCount,
  })

  function onScroll(event: UIEvent<HTMLDivElement>) {
    if (
      event.currentTarget.scrollTop < LOAD_OLDER_THRESHOLD_PX &&
      hasNextPage &&
      !isFetchingNextPage
    ) {
      void fetchNextPage()
    }
  }

  if (query.isPending) return <ListSkeleton />
  if (query.error) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-sm">
        <p className="text-destructive">Couldn't load messages: {query.error.message}</p>
        <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
          Try again
        </Button>
      </div>
    )
  }

  const isGroup = chat.kind === "group"

  return (
    <JumpToMessageContext value={(id) => void jumpTo(id)}>
      <MessageScroller className="min-h-0 flex-1">
        <MessageScrollerViewport onScroll={onScroll}>
          <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-1 px-5 pt-4 pb-3">
            {!hasNextPage && messages.length === 0 && pending.length === 0 && (
              <div className="m-auto flex flex-col items-center gap-1 text-center select-none">
                <p className="text-[15px] font-medium">Say hi to {chat.name}</p>
                <p className="max-w-72 text-[13px] text-muted-foreground">
                  {isGroup
                    ? "Everyone here sees the chat. @mention an agent to ask it directly."
                    : "Ask for anything. It remembers what matters and can work on its own."}
                </p>
              </div>
            )}
            {rows.map((row, i) => (
              <MessageScrollerItem
                key={rowKey(row.message.id)}
                messageId={rowKey(row.message.id)}
                scrollAnchor={pending.length === 0 && row.message.id === lastUserId}
                className={cn(
                  row.startsRun && i > 0 && "mt-5",
                  flashId === row.message.id && "reply-flash",
                )}
              >
                {row.startsDay && <DaySeparator iso={row.message.createdAt} />}
                {row.message.id === unreadFrom && <UnreadDivider />}
                <MessageRow
                  row={row}
                  agents={agents}
                  showAuthorName={isGroup}
                  isLatest={i === rows.length - 1 && pending.length === 0}
                />
              </MessageScrollerItem>
            ))}
            {pending.map((p, i) => (
              <MessageScrollerItem
                key={p.clientId}
                messageId={p.clientId}
                scrollAnchor={i === pending.length - 1}
                className={cn((i > 0 || rows.length > 0) && "mt-5")}
              >
                <PendingRow
                  pending={p}
                  agents={agents}
                  onRetry={() => retry(p)}
                  onDiscard={() => discard(p.clientId)}
                />
              </MessageScrollerItem>
            ))}
            <div className="mt-3 empty:mt-0">
              <ActivityIndicator agents={members} named={isGroup} />
            </div>
          </MessageScrollerContent>
        </MessageScrollerViewport>
        {isFetchingNextPage && (
          <div
            className="pointer-events-none absolute inset-x-0 top-3 flex justify-center"
            role="status"
            aria-label="Loading older messages"
          >
            <span className="rounded-full border bg-background p-1.5 shadow-sm">
              <Spinner />
            </span>
          </div>
        )}
        {/* With unread messages below, it says how many. */}
        <MessageScrollerButton
          direction="end"
          className={cn(
            "rounded-full border bg-popover shadow-[0_6px_20px_rgb(0_0_0/18%)] hover:bg-accent",
            chat.unreadCount > 0
              ? "h-8 w-auto gap-1.5 border-transparent bg-brand px-3 text-xs font-medium text-white hover:bg-brand/90"
              : "size-9",
          )}
        >
          <ArrowDownIcon className="size-4" />
          {chat.unreadCount > 0 ? (
            <span className="tabular-nums">{chat.unreadCount} new</span>
          ) : (
            <span className="sr-only">Scroll to end</span>
          )}
        </MessageScrollerButton>
      </MessageScroller>
    </JumpToMessageContext>
  )
}

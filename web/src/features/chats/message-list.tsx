import { useInfiniteQuery } from "@tanstack/react-query"
import { cn } from "cn"
import { useMemo, type UIEvent } from "react"

import { Button } from "@/components/ui/button"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import type { Agent, Chat } from "@/lib/api-client"
import { flattenMessages } from "@/lib/chat-cache"

import { messagesQueryOptions, useSendMessage } from "./api"
import { toRows } from "./grouping"
import { DaySeparator, MessageRow, PendingRow } from "./message-row"
import { reconcilePending, usePendingMessages } from "./pending-store"
import { useMarkReadWhenSeen } from "./read-tracker"

/** Start loading older history when this close to the top. */
const LOAD_OLDER_THRESHOLD_PX = 400

function ListSkeleton() {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-6" aria-hidden="true">
      <Skeleton className="h-10 w-2/3 rounded-xl" />
      <Skeleton className="h-10 w-1/2 self-end rounded-xl" />
      <Skeleton className="h-20 w-3/4 rounded-xl" />
    </div>
  )
}

/** Must be rendered inside a MessageScrollerProvider keyed by chat id. */
export function MessageList({ chat, agents }: { chat: Chat; agents: Map<string, Agent> }) {
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
    <MessageScroller className="min-h-0 flex-1">
      <MessageScrollerViewport onScroll={onScroll}>
        <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-1 px-4 pt-6 pb-2">
          {!hasNextPage && messages.length === 0 && pending.length === 0 && (
            <p className="m-auto text-sm text-muted-foreground select-none">
              Say hi to {chat.name}.
            </p>
          )}
          {rows.map((row, i) => (
            <MessageScrollerItem
              key={rowKey(row.message.id)}
              messageId={rowKey(row.message.id)}
              scrollAnchor={pending.length === 0 && row.message.id === lastUserId}
              className={cn(row.startsRun && i > 0 && "mt-4")}
            >
              {row.startsDay && <DaySeparator iso={row.message.createdAt} />}
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
              className={cn((i > 0 || rows.length > 0) && "mt-4")}
            >
              <PendingRow
                pending={p}
                onRetry={() => retry(p)}
                onDiscard={() => discard(p.clientId)}
              />
            </MessageScrollerItem>
          ))}
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
      <MessageScrollerButton direction="end" />
    </MessageScroller>
  )
}

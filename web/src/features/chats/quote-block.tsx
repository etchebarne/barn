import { cn } from "cn"
import { FileIcon, ReplyIcon } from "lucide-react"
import { createContext, useContext } from "react"

import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Agent, Message } from "@/lib/api-client"
import { isImage } from "@/lib/attachments"

import { agentAuthorName } from "./preview"
import { quoteExcerpt, useReplyStore, type QuotedMessage } from "./reply-store"

/** Scrolls to a message (loading older history if needed) and flashes it. */
export const JumpToMessageContext = createContext<(messageId: string) => void>(() => {})

/** "You", the agent's name, "Deleted agent" or "System". */
export function quoteAuthorName(
  author: QuotedMessage["author"],
  agents: Map<string, Agent>,
): string {
  if (!author) return "Someone"
  if (author.kind === "user") return "You"
  if (author.kind === "system") return "System"
  return agentAuthorName(author.agentId, agents).name
}

/**
 * The message a reply quotes, shown above it: a thin rule, the author, a plain-text excerpt,
 * and a tiny image thumbnail or file name. Clicking it jumps to the original.
 */
export function QuoteBlock({
  quote,
  agents,
  align,
  className,
}: {
  quote: QuotedMessage
  agents: Map<string, Agent>
  align: "start" | "end"
  className?: string
}) {
  const jump = useContext(JumpToMessageContext)
  const base = cn(
    "flex max-w-[min(80%,24rem)] min-w-0 items-center gap-2 rounded-md border-l-2 bg-muted/60 py-1 pr-2 pl-2 text-left text-xs",
    align === "end" ? "self-end" : "self-start",
    className,
  )

  if (!quote.available) {
    return (
      <div className={cn(base, "border-muted-foreground/30 text-muted-foreground italic")}>
        Original message deleted
      </div>
    )
  }

  const name = quoteAuthorName(quote.author, agents)
  const excerpt = quoteExcerpt(quote)
  const attachments = quote.attachments ?? []
  const image = attachments.find((a) => isImage(a.mime))
  const file = image ? undefined : attachments[0]

  return (
    <button
      type="button"
      className={cn(
        base,
        "border-primary/50 outline-none select-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50",
      )}
      aria-label={`Reply to ${name}${excerpt ? `: “${excerpt}”` : ""}. Show the original message`}
      onClick={() => jump(quote.id)}
    >
      {image && (
        <img
          src={image.url}
          alt=""
          loading="lazy"
          className="size-8 shrink-0 rounded object-cover"
        />
      )}
      {file && <FileIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />}
      <span className="flex min-w-0 flex-col">
        <span className="truncate font-medium">{name}</span>
        {excerpt && <span className="line-clamp-2 text-muted-foreground">{excerpt}</span>}
      </span>
    </button>
  )
}

/** "Reply" in a message's hover actions: starts a reply in the composer. */
export function ReplyButton({ message }: { message: Message }) {
  const startReply = useReplyStore((s) => s.startReply)
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Reply"
            className="text-muted-foreground"
            onClick={() => startReply(message)}
          />
        }
      >
        <ReplyIcon />
      </TooltipTrigger>
      <TooltipContent>Reply</TooltipContent>
    </Tooltip>
  )
}

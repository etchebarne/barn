import { cn } from "cn"
import { SmilePlusIcon } from "lucide-react"
import { useState } from "react"

import { BubbleReactions } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Agent, Message } from "@/lib/api-client"
import { QUICK_REACTIONS, userReacted, type Reaction } from "@/lib/reactions"

import { useToggleReaction } from "./api"

/** "barn", "barn and Tracker", "barn, Tracker and you". */
export function reactorNames(reaction: Reaction, agents: Map<string, Agent>): string {
  const names = reaction.by.map((author) =>
    author.kind === "agent" ? (agents.get(author.agentId ?? "")?.name ?? "An agent") : "you",
  )
  if (names.length <= 1) return names[0] ?? ""
  return `${names.slice(0, -1).join(", ")} and ${names.at(-1)}`
}

/**
 * Reactions pinned to the bottom corner of a bubble, on the side away from the author's
 * avatar. Each emoji shows its count when more than one person used it; hovering names who
 * reacted; clicking toggles your own reaction. Appears without animation (frequent action).
 */
export function MessageReactions({
  message,
  agents,
  align,
}: {
  message: Message
  agents: Map<string, Agent>
  align: "start" | "end"
}) {
  const toggle = useToggleReaction(message)
  if (message.reactions.length === 0) return null
  return (
    <BubbleReactions align={align} className="ring-background select-none has-[button]:p-0.5">
      {message.reactions.map((reaction) => {
        const who = reactorNames(reaction, agents)
        const mine = userReacted(reaction)
        return (
          <Tooltip key={reaction.emoji}>
            <TooltipTrigger
              render={
                <button
                  type="button"
                  aria-label={`${who} reacted ${reaction.emoji}. ${mine ? "Remove" : "Add"} your reaction`}
                />
              }
              className={cn(
                "flex h-5 items-center gap-0.5 rounded-full px-1 leading-none outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                "transition-transform duration-(--duration-press) ease-out-quint active:scale-90",
                mine ? "bg-primary/15 ring-1 ring-primary/40" : "hover:bg-background/60",
              )}
              aria-pressed={mine}
              onClick={() => toggle.mutate(reaction.emoji)}
            >
              <span>{reaction.emoji}</span>
              {reaction.by.length > 1 ? (
                <span className="text-xs text-muted-foreground tabular-nums">
                  {reaction.by.length}
                </span>
              ) : null}
            </TooltipTrigger>
            <TooltipContent>{`${who} reacted ${reaction.emoji}`}</TooltipContent>
          </Tooltip>
        )
      })}
    </BubbleReactions>
  )
}

/** "React" hover action: a small popover with quick emojis that toggle your reaction. */
export function ReactButton({ message }: { message: Message }) {
  const [open, setOpen] = useState(false)
  const toggle = useToggleReaction(message)
  const mine = new Set(message.reactions.filter(userReacted).map((r) => r.emoji))
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="React"
            className="text-muted-foreground"
          />
        }
      >
        <SmilePlusIcon />
      </PopoverTrigger>
      <PopoverContent side="top" className="w-auto flex-row gap-0.5 p-1" aria-label="Reactions">
        {QUICK_REACTIONS.map((emoji) => (
          <Button
            key={emoji}
            variant="ghost"
            size="icon"
            aria-label={`React ${emoji}`}
            aria-pressed={mine.has(emoji)}
            className={cn("text-base", mine.has(emoji) && "bg-primary/15")}
            onClick={() => {
              toggle.mutate(emoji)
              setOpen(false)
            }}
          >
            {emoji}
          </Button>
        ))}
      </PopoverContent>
    </Popover>
  )
}

import { BubbleReactions } from "@/components/ui/bubble"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { Agent, Schemas } from "@/lib/api-client"

type Reaction = Schemas["Reaction"]

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
 * reacted.
 */
export function MessageReactions({
  reactions,
  agents,
  align,
}: {
  reactions: Reaction[]
  agents: Map<string, Agent>
  align: "start" | "end"
}) {
  if (reactions.length === 0) return null
  return (
    <BubbleReactions align={align} className="ring-background select-none">
      {reactions.map((reaction) => {
        const who = reactorNames(reaction, agents)
        return (
          <Tooltip key={reaction.emoji}>
            <TooltipTrigger
              render={<span />}
              className="flex items-center gap-0.5 leading-none"
              aria-label={`${who} reacted ${reaction.emoji}`}
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

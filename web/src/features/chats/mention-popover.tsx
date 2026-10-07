import { cn } from "cn"
import type { RefObject } from "react"

import { Popover, PopoverContent } from "@/components/ui/popover"
import { AgentAvatar } from "@/features/agents"
import type { MentionTarget } from "@/lib/mentions"

export function mentionOptionId(listId: string, index: number) {
  return `${listId}-${index}`
}

/**
 * The composer's @mention suggestions. Focus stays in the textarea (the composer drives the
 * list with the keyboard); moving the highlight is not animated.
 */
export function MentionPopover({
  open,
  anchor,
  listId,
  candidates,
  activeIndex,
  onPick,
  onHover,
  onDismiss,
}: {
  open: boolean
  anchor: RefObject<HTMLElement | null>
  listId: string
  candidates: MentionTarget[]
  activeIndex: number
  onPick: (candidate: MentionTarget) => void
  onHover: (index: number) => void
  onDismiss: () => void
}) {
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (!next) onDismiss()
      }}
    >
      <PopoverContent
        anchor={anchor}
        side="top"
        align="start"
        sideOffset={6}
        initialFocus={false}
        finalFocus={false}
        className="w-64 gap-0 p-1"
      >
        <div role="listbox" id={listId} aria-label="Mention an agent">
          {candidates.map((candidate, index) => (
            <div
              key={candidate.id}
              id={mentionOptionId(listId, index)}
              role="option"
              tabIndex={-1}
              aria-selected={index === activeIndex}
              className={cn(
                "flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm select-none",
                index === activeIndex && "bg-accent text-accent-foreground",
              )}
              // Keep focus (and the caret) in the textarea.
              onMouseDown={(event) => event.preventDefault()}
              onMouseMove={() => onHover(index)}
              onClick={() => onPick(candidate)}
              onKeyDown={() => {}}
            >
              <AgentAvatar id={candidate.id} name={candidate.name} size="sm" />
              <span className="truncate">{candidate.name}</span>
            </div>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

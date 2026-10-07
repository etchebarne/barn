import { ArrowUpIcon } from "lucide-react"
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type SyntheticEvent,
} from "react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import {
  activeMentionQuery,
  filterMentionCandidates,
  insertMention,
  type MentionTarget,
} from "@/lib/mentions"
import { shake } from "@/lib/shake"

import { MentionPopover, mentionOptionId } from "./mention-popover"

/** The request schema caps message bodies at 32k characters. */
export const MAX_MESSAGE_LENGTH = 32_000

/**
 * Message composer: grows with its content (`field-sizing: content`) up to a max height.
 * Enter sends, Shift+Enter inserts a newline. Sending is never animated.
 *
 * With `mentionCandidates` (group chats), typing `@` suggests agents: arrows move, Enter or Tab
 * inserts `@Name `, Escape closes. While suggestions are open, Enter never sends.
 */
export function Composer({
  value,
  onChange,
  onSend,
  placeholder,
  mentionCandidates,
}: {
  value: string
  onChange: (value: string) => void
  onSend: (text: string) => void
  placeholder?: string
  mentionCandidates?: MentionTarget[]
}) {
  const groupRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const listId = useId()
  const tooLong = value.length > MAX_MESSAGE_LENGTH

  // @mention autocomplete state.
  const [caret, setCaret] = useState(0)
  const [focused, setFocused] = useState(false)
  const [dismissedAt, setDismissedAt] = useState<number | null>(null)
  const [highlight, setHighlight] = useState({ key: "", index: 0 })
  const mention = mentionCandidates ? activeMentionQuery(value, caret) : null
  const matches =
    mention && mentionCandidates ? filterMentionCandidates(mentionCandidates, mention.query) : []
  // Suggestions only show while typing here; clicking back into the textarea keeps them.
  const mentionOpen =
    focused && mention !== null && matches.length > 0 && dismissedAt !== mention.start
  const highlightKey = mention ? `${mention.start}:${mention.query}` : ""
  const activeIndex =
    highlight.key === highlightKey ? Math.min(highlight.index, matches.length - 1) : 0

  function trackCaret(event: SyntheticEvent<HTMLTextAreaElement>) {
    setCaret(event.currentTarget.selectionStart)
  }

  function pickMention(target: MentionTarget) {
    if (!mention) return
    const next = insertMention(value, mention.start, caret, target.name)
    onChange(next.text)
    setCaret(next.caret)
    pendingCaret.current = next.caret
  }

  // Place the caret after an inserted mention as soon as the new value is in the DOM.
  const pendingCaret = useRef<number | null>(null)
  useLayoutEffect(() => {
    const textarea = textareaRef.current
    if (pendingCaret.current === null || !textarea) return
    textarea.focus()
    textarea.setSelectionRange(pendingCaret.current, pendingCaret.current)
    pendingCaret.current = null
  })

  /** Handles keys while suggestions are open. Returns true if the key was used. */
  function onMentionKey(event: KeyboardEvent<HTMLTextAreaElement>): boolean {
    if (!mentionOpen || !mention) return false
    const count = matches.length
    switch (event.key) {
      case "ArrowDown":
        setHighlight({ key: highlightKey, index: (activeIndex + 1) % count })
        return true
      case "ArrowUp":
        setHighlight({ key: highlightKey, index: (activeIndex - 1 + count) % count })
        return true
      case "Enter":
      case "Tab": {
        if (event.shiftKey || event.nativeEvent.isComposing) return false
        const target = matches[activeIndex]
        if (target) pickMention(target)
        return true
      }
      case "Escape":
        // Close the suggestions only; don't let Escape close anything else.
        event.stopPropagation()
        setDismissedAt(mention.start)
        return true
      default:
        return false
    }
  }

  function submit(fromPointer: boolean) {
    const text = value.trim()
    if (!text || tooLong) {
      // Explain the blocked action with a small shake, but only for pointer input:
      // keyboard actions are never animated.
      if (fromPointer) shake(groupRef.current)
      return
    }
    onSend(text)
    onChange("")
    textareaRef.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (onMentionKey(event)) {
      event.preventDefault()
      return
    }
    if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return
    event.preventDefault()
    submit(false)
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        submit(true)
      }}
    >
      {/* Nested radius: group radius = button radius (--radius) + addon padding (0.5rem). */}
      <InputGroup ref={groupRef} className="rounded-[calc(var(--radius)+0.5rem)] bg-background">
        <InputGroupTextarea
          ref={textareaRef}
          aria-label="Message"
          placeholder={placeholder}
          rows={1}
          value={value}
          aria-invalid={tooLong || undefined}
          onChange={(event) => {
            onChange(event.target.value)
            setCaret(event.target.selectionStart)
          }}
          onSelect={trackCaret}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          onKeyDown={onKeyDown}
          {...(mentionCandidates
            ? {
                role: "combobox",
                "aria-autocomplete": "list" as const,
                "aria-expanded": mentionOpen,
                "aria-controls": mentionOpen ? listId : undefined,
                "aria-activedescendant": mentionOpen
                  ? mentionOptionId(listId, activeIndex)
                  : undefined,
              }
            : {})}
          className="max-h-[40svh] min-h-11 px-3.5 pt-3"
        />
        <InputGroupAddon align="block-end" className="justify-between">
          <span className="text-xs text-destructive tabular-nums" aria-live="polite">
            {tooLong
              ? `${value.length.toLocaleString()} / ${MAX_MESSAGE_LENGTH.toLocaleString()}`
              : ""}
          </span>
          <InputGroupButton
            type="submit"
            variant="default"
            size="icon-sm"
            className="rounded-(--radius)"
            aria-label="Send message"
            aria-disabled={!value.trim() || tooLong || undefined}
          >
            <ArrowUpIcon />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
      {mentionCandidates && (
        <MentionPopover
          open={mentionOpen}
          anchor={groupRef}
          listId={listId}
          candidates={matches}
          activeIndex={activeIndex}
          onPick={pickMention}
          onHover={(index) => setHighlight({ key: highlightKey, index })}
          // Outside presses blur the textarea, which closes the list; Escape is handled above.
          onDismiss={() => {}}
        />
      )}
    </form>
  )
}

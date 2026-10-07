import { ArrowUpIcon } from "lucide-react"
import { useRef, type KeyboardEvent } from "react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { shake } from "@/lib/shake"

/** The request schema caps message bodies at 32k characters. */
export const MAX_MESSAGE_LENGTH = 32_000

/**
 * Message composer: grows with its content (`field-sizing: content`) up to a max height.
 * Enter sends, Shift+Enter inserts a newline. Sending is never animated.
 */
export function Composer({
  value,
  onChange,
  onSend,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  onSend: (text: string) => void
  placeholder?: string
}) {
  const groupRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const tooLong = value.length > MAX_MESSAGE_LENGTH

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
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={onKeyDown}
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
    </form>
  )
}

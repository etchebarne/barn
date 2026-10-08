import { cn } from "cn"
import { ArrowUpIcon, PaperclipIcon, ReplyIcon, XIcon } from "lucide-react"
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
  type SyntheticEvent,
} from "react"

import { Button } from "@/components/ui/button"
import {
  activeMentionQuery,
  filterMentionCandidates,
  insertMention,
  type MentionTarget,
} from "@/lib/mentions"
import { shake } from "@/lib/shake"

import { AttachmentRow } from "./attachment-row"
import { MentionPopover, mentionOptionId } from "./mention-popover"
import { uploadsState, type PendingUpload } from "./uploads-store"

const NO_UPLOADS: PendingUpload[] = []

/** Files on the clipboard (screenshots, or files copied in a file manager). */
export function clipboardFiles(data: DataTransfer | null): File[] {
  if (!data) return []
  const files = [...data.files]
  if (files.length > 0) return files
  return [...data.items]
    .filter((item) => item.kind === "file")
    .flatMap((item) => {
      const file = item.getAsFile()
      return file ? [file] : []
    })
}

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
  uploads = NO_UPLOADS,
  uploadNotice = null,
  onAddFiles,
  onRemoveUpload,
  onRetryUpload,
  replyTo = null,
  onCancelReply,
  focusRequest = 0,
}: {
  value: string
  onChange: (value: string) => void
  /** Called with the trimmed text (may be empty when files are attached). */
  onSend: (text: string) => void
  placeholder?: string
  mentionCandidates?: MentionTarget[]
  /** Files added to this message; enables attaching when `onAddFiles` is set. */
  uploads?: PendingUpload[]
  uploadNotice?: string | null
  onAddFiles?: (files: File[]) => void
  onRemoveUpload?: (localId: string) => void
  onRetryUpload?: (localId: string) => void
  /** The message being replied to, shown above the input with a cancel button. */
  replyTo?: { name: string; excerpt: string } | null
  onCancelReply?: () => void
  /** Changes when something (e.g. Reply) wants the composer focused. */
  focusRequest?: number
}) {
  const groupRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const listId = useId()
  const tooLong = value.length > MAX_MESSAGE_LENGTH
  const { uploading, ready } = uploadsState(uploads)
  // Text or a finished upload, nothing still uploading, and not over the length limit.
  const canSend = (value.trim() !== "" || ready > 0) && !uploading && !tooLong

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

  // Reply (or anything else) asked for focus. Only changes after mount count, so opening a chat
  // doesn't grab focus (and pop the keyboard on phones).
  const seenFocusRequest = useRef(focusRequest)
  useEffect(() => {
    if (focusRequest === seenFocusRequest.current) return
    seenFocusRequest.current = focusRequest
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.focus()
    // After the draft, so a started sentence ("Make me a new agent that ") continues.
    textarea.setSelectionRange(textarea.value.length, textarea.value.length)
  }, [focusRequest])

  // Compact until the text wraps (or has a newline, or files are attached); then expanded until
  // it's cleared, so the layout doesn't flip back and forth at the wrap point.
  const [wrapped, setWrapped] = useState(false)
  const expanded = (value !== "" && (wrapped || value.includes("\n"))) || uploads.length > 0

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
    if (!canSend) {
      // Explain the blocked action with a small shake, but only for pointer input:
      // keyboard actions are never animated.
      if (fromPointer) shake(groupRef.current)
      return
    }
    onSend(text)
    onChange("")
    setWrapped(false)
    textareaRef.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (onMentionKey(event)) {
      event.preventDefault()
      return
    }
    if (event.key === "Escape" && replyTo && onCancelReply) {
      event.preventDefault()
      event.stopPropagation()
      onCancelReply()
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
      {/* What the message is attached to sits on a slab tucked under the composer's top edge, so
          the two read as one control. */}
      {replyTo && (
        <div
          role="status"
          aria-label={`Replying to ${replyTo.name}`}
          className="mx-3 -mb-3.5 flex min-w-0 items-center gap-2 rounded-t-xl border border-b-0 bg-muted px-3 pt-1.5 pb-5 text-xs"
        >
          <ReplyIcon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate">
            <span className="text-muted-foreground">Replying to </span>
            <span className="font-medium">{replyTo.name}</span>
            {replyTo.excerpt && <span className="text-muted-foreground"> · {replyTo.excerpt}</span>}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="-mr-1.5"
            aria-label="Cancel reply"
            onClick={() => {
              onCancelReply?.()
              textareaRef.current?.focus()
            }}
          >
            <XIcon />
          </Button>
        </div>
      )}
      {/* Compact: one line, [attach] [text] [send]. Expanded (the text wraps, or files are
          attached): the text gets the full width and the buttons move to a row under it. Same
          elements either way, so focus and the caret survive the switch. Nested radius: 24px =
          the round buttons' radius (16) + the padding (8). */}
      <div
        ref={groupRef}
        role="group"
        data-expanded={expanded || undefined}
        className={cn(
          "relative grid cursor-text grid-cols-[auto_minmax(0,1fr)_auto] items-end gap-x-1 rounded-[24px] border bg-surface p-2 shadow-[0_1px_2px_rgb(0_0_0/4%)] transition-[border-color,box-shadow] duration-(--duration-press)",
          "has-[textarea:focus-visible]:border-foreground/20 has-[textarea:focus-visible]:ring-3 has-[textarea:focus-visible]:ring-foreground/5",
          tooLong && "border-destructive/60",
        )}
        onPointerDown={(event) => {
          // Clicks on the padding focus the text, like a native field.
          if (event.target === event.currentTarget) {
            event.preventDefault()
            textareaRef.current?.focus()
          }
        }}
      >
        <div
          className={cn(
            "col-span-3",
            !expanded && uploads.length === 0 && !uploadNotice && "hidden",
          )}
        >
          <AttachmentRow
            uploads={uploads}
            notice={uploadNotice}
            onRemove={(id) => onRemoveUpload?.(id)}
            onRetry={(id) => onRetryUpload?.(id)}
          />
        </div>
        {onAddFiles ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={cn(
              "rounded-full text-muted-foreground",
              expanded ? "row-start-3" : "row-start-2",
            )}
            aria-label="Attach files"
            onClick={() => fileInputRef.current?.click()}
          >
            <PaperclipIcon />
          </Button>
        ) : (
          <span className={expanded ? "row-start-3" : "row-start-2"} />
        )}
        <textarea
          ref={textareaRef}
          aria-label="Message"
          placeholder={placeholder}
          rows={1}
          value={value}
          aria-invalid={tooLong || undefined}
          onChange={(event) => {
            const next = event.target.value
            onChange(next)
            setCaret(event.target.selectionStart)
            // The DOM already holds the new text, so its height says whether it wraps.
            setWrapped(next !== "" && (wrapped || event.target.scrollHeight > 40))
          }}
          onSelect={trackCaret}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          onKeyDown={onKeyDown}
          onPaste={(event: ClipboardEvent<HTMLTextAreaElement>) => {
            if (!onAddFiles) return
            const files = clipboardFiles(event.clipboardData)
            if (files.length === 0) return
            // Attach the files instead of pasting their names or nothing.
            event.preventDefault()
            onAddFiles(files)
          }}
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
          className={cn(
            "field-sizing-content max-h-[40svh] min-h-8 w-full resize-none bg-transparent px-1.5 py-1.5 text-sm leading-5 outline-none placeholder:text-muted-foreground",
            expanded ? "col-span-3 row-start-2 px-2 pb-2" : "row-start-2",
          )}
        />
        <div
          className={cn(
            "col-start-3 flex items-center gap-2",
            expanded ? "row-start-3" : "row-start-2",
          )}
        >
          {tooLong && (
            <span className="text-xs text-destructive tabular-nums" aria-live="polite">
              {value.length.toLocaleString()} / {MAX_MESSAGE_LENGTH.toLocaleString()}
            </span>
          )}
          <Button
            type="submit"
            size="icon"
            className={cn("rounded-full", !canSend && "opacity-40")}
            aria-label="Send message"
            aria-disabled={!canSend || undefined}
          >
            <ArrowUpIcon className="size-4.5" strokeWidth={2.25} />
          </Button>
        </div>
      </div>
      {onAddFiles && (
        <input
          ref={fileInputRef}
          type="file"
          multiple
          hidden
          aria-hidden="true"
          tabIndex={-1}
          data-testid="attach-input"
          onChange={(event) => {
            onAddFiles([...(event.target.files ?? [])])
            // Allow picking the same file again later.
            event.target.value = ""
          }}
        />
      )}
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

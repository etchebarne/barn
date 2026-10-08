import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { cn } from "cn"
import { CheckIcon, PencilIcon, SendHorizontalIcon, XIcon } from "lucide-react"
import { useEffect, useEffectEvent, useState, type FormEvent, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { useConnectorTypes } from "@/features/connectors"
import { agentsQueryOptions } from "@/lib/agents"
import { ApiError, type Message } from "@/lib/api-client"

import { ActionPreviewCard } from "./action-preview-card"
import {
  ensureChatLoaded,
  useAnswerPrompt,
  useConnectPrompt,
  useDismissPrompt,
  useProvideSecret,
} from "./api"
import { DECLINE_CONNECT } from "./connect"
import { ConnectCard } from "./connect-card"
import {
  acceptsLetterKeys,
  alwaysAllowAnswer,
  approvalAnswer,
  approvalOutcome,
  offersAlwaysAllow,
  buildAnswer,
  chatToOpen,
  letterIndex,
  optionLetter,
  promptRows,
  type Prompt,
  type PromptAnswer,
} from "./logic"
import { DECLINE_SECRET, SecretCard } from "./secret-card"

/*
 * Radii follow the nested rule: the bubble (radius-xl) pads the card by 0.5rem, so rows get
 * radius-xl - 0.5rem; rows pad their badge by 0.375rem, so badges get what's left (min 4px).
 */
const ROW_RADIUS = "rounded-[calc(var(--radius-xl)-0.5rem)]"
const BADGE_RADIUS = "rounded-[max(4px,calc(var(--radius-xl)-0.875rem))]"
const ROW = "flex w-full min-w-0 items-center gap-2.5 px-1.5 py-1.5 text-left text-sm " + ROW_RADIUS

/** Press feedback only: no hover transition, no entrance animation (cards are frequent). */
const PRESSABLE =
  "cursor-pointer select-none outline-none transition-transform duration-(--duration-press) ease-out-quint hover:bg-background/70 focus-visible:ring-2 focus-visible:ring-ring/50 active:scale-[0.985] disabled:pointer-events-none"

function Badge({ children, active }: { children: ReactNode; active?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-6 shrink-0 items-center justify-center border text-xs font-medium tabular-nums",
        BADGE_RADIUS,
        active ? "border-primary bg-primary text-primary-foreground" : "bg-background",
      )}
    >
      {children}
    </span>
  )
}

function isTypingTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    target.closest("input, textarea, select, [contenteditable='true'], [role='dialog']") !== null
  )
}

/** Inline "type your answer" field used by text prompts and "Type your own…". */
function TextAnswer({
  placeholder,
  disabled,
  autoFocus,
  onSubmit,
  value,
  onChange,
  showSend = true,
}: {
  placeholder: string
  disabled?: boolean
  autoFocus?: boolean
  onSubmit: () => void
  value: string
  onChange: (value: string) => void
  showSend?: boolean
}) {
  return (
    <InputGroup className={cn("h-9 bg-background", ROW_RADIUS)}>
      <InputGroupInput
        aria-label={placeholder}
        placeholder={placeholder}
        autoFocus={autoFocus}
        disabled={disabled}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault()
            onSubmit()
          }
        }}
      />
      {showSend && (
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            size="icon-xs"
            aria-label="Send answer"
            disabled={disabled || !value.trim()}
            onClick={onSubmit}
          >
            <SendHorizontalIcon />
          </InputGroupButton>
        </InputGroupAddon>
      )}
    </InputGroup>
  )
}

function AnsweredCard({ prompt }: { prompt: Prompt }) {
  const rows = promptRows(prompt)
  return (
    <ul className="flex flex-col gap-0.5 opacity-70" aria-label="Your answer">
      {rows.map((row) =>
        row.type === "option" ? (
          <li key={row.index} className={ROW}>
            <Badge>{optionLetter(row.index)}</Badge>
            <span className="min-w-0 flex-1 wrap-break-word">{row.label}</span>
            <CheckIcon className="size-4 shrink-0" aria-label="Chosen" />
          </li>
        ) : (
          <li key="text" className={ROW}>
            <Badge>
              <PencilIcon className="size-3" />
            </Badge>
            <span className="min-w-0 flex-1 wrap-break-word whitespace-pre-wrap">{row.text}</span>
            <CheckIcon className="size-4 shrink-0" aria-label="Chosen" />
          </li>
        ),
      )}
    </ul>
  )
}

function PendingCard({
  message,
  prompt,
  isLatest,
  onAnswer,
  disabled,
}: {
  message: Message
  prompt: Prompt
  isLatest: boolean
  onAnswer: (answer: PromptAnswer) => void
  disabled: boolean
}) {
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set())
  const [otherOpen, setOtherOpen] = useState(false)
  const [text, setText] = useState("")
  const multi = prompt.kind === "multi"

  /**
   * Picking an option only selects it; nothing is sent until Submit (or Enter), so a stray
   * click or keypress can't answer. Single choice keeps one selection, and an option and a
   * typed answer replace each other.
   */
  function choose(index: number) {
    if (disabled) return
    setSelected((current) => {
      if (multi) {
        const next = new Set(current)
        if (next.has(index)) next.delete(index)
        else next.add(index)
        return next
      }
      return current.has(index) ? new Set() : new Set([index])
    })
    if (!multi) setText("")
  }

  function changeText(value: string) {
    setText(value)
    if (!multi && value.trim()) setSelected(new Set())
  }

  const answer = buildAnswer(selected, text)
  function submit() {
    if (answer && !disabled) onAnswer(answer)
  }

  // Letter keys select options on the latest card, unless the user is typing somewhere.
  // Enter (outside text fields, which handle it themselves) submits a single-choice selection.
  const onLetter = useEffectEvent((index: number) => choose(index))
  const onEnter = useEffectEvent(() => submit())
  const keysActive = acceptsLetterKeys(prompt, isLatest)
  const optionCount = prompt.options.length
  useEffect(() => {
    if (!keysActive) return undefined
    function onKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
      if (isTypingTarget(event.target)) return
      if (event.key === "Enter" && !multi) {
        // Focused buttons handle their own Enter (options submit; others act normally).
        if (event.target instanceof HTMLButtonElement) return
        event.preventDefault()
        onEnter()
        return
      }
      const index = letterIndex(event.key, optionCount)
      if (index === null) return
      event.preventDefault()
      onLetter(index)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [keysActive, optionCount, multi])

  if (prompt.kind === "text") {
    return (
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault()
          submit()
        }}
      >
        <TextAnswer
          placeholder="Type your answer"
          disabled={disabled}
          value={text}
          onChange={setText}
          onSubmit={submit}
        />
      </form>
    )
  }

  const canSubmit = answer !== null

  return (
    <div className="flex flex-col gap-0.5">
      <div role="group" aria-label={message.body} className="flex flex-col gap-0.5">
        {prompt.options.map((option, index) => {
          const isOn = selected.has(index)
          return (
            <button
              // oxlint-disable-next-line react/no-array-index-key -- options are positional
              key={index}
              type="button"
              disabled={disabled}
              aria-pressed={isOn}
              className={cn(ROW, PRESSABLE, isOn && "bg-background ring-1 ring-border")}
              onClick={() => choose(index)}
              onKeyDown={(event) => {
                // On a single-choice option, Enter submits rather than toggling it again.
                if (!multi && event.key === "Enter") {
                  event.preventDefault()
                  submit()
                }
              }}
            >
              <Badge active={isOn}>{optionLetter(index)}</Badge>
              <span className="min-w-0 flex-1 wrap-break-word">{option.label}</span>
              {isOn && <CheckIcon className="size-4 shrink-0" aria-hidden="true" />}
            </button>
          )
        })}
        {prompt.allowOther &&
          (otherOpen ? (
            <div className="px-0.5 pt-0.5">
              <TextAnswer
                placeholder="Type your own…"
                autoFocus
                disabled={disabled}
                value={text}
                onChange={changeText}
                onSubmit={submit}
                showSend={false}
              />
            </div>
          ) : (
            <button
              type="button"
              disabled={disabled}
              className={cn(ROW, PRESSABLE, "text-muted-foreground")}
              onClick={() => setOtherOpen(true)}
            >
              <Badge>
                <PencilIcon className="size-3" />
              </Badge>
              Type your own…
            </button>
          ))}
      </div>
      <div className="flex justify-end pt-1.5">
        <Button size="sm" disabled={disabled || !canSubmit} onClick={submit}>
          Submit
        </Button>
      </div>
    </div>
  )
}

/**
 * An agent asking permission for a gated action: Approve (primary) or Decline. No keyboard
 * shortcuts, so nothing gets approved by accident.
 */
function ApprovalActions({
  onAnswer,
  disabled,
  alwaysAllow,
}: {
  onAnswer: (answer: PromptAnswer) => void
  disabled: boolean
  alwaysAllow: boolean
}) {
  return (
    <div className="flex flex-wrap justify-end gap-2 px-1.5 pb-1">
      {alwaysAllow && (
        <Button
          size="sm"
          variant="ghost"
          className="mr-auto text-muted-foreground"
          disabled={disabled}
          onClick={() => onAnswer(alwaysAllowAnswer())}
        >
          Always allow
        </Button>
      )}
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => onAnswer(approvalAnswer(false))}
      >
        Decline
      </Button>
      <Button size="sm" disabled={disabled} onClick={() => onAnswer(approvalAnswer(true))}>
        Approve
      </Button>
    </div>
  )
}

function ApprovalOutcome({ prompt }: { prompt: Prompt }) {
  const outcome = approvalOutcome(prompt)
  if (!outcome) return null
  return (
    <p className="flex items-center gap-1.5 px-1.5 pb-1 text-xs font-medium text-muted-foreground">
      {outcome === "approved" || outcome === "always" ? (
        <>
          <CheckIcon className="size-3.5" aria-hidden="true" />
          {outcome === "always" ? "Always allowed" : "Approved"}
        </>
      ) : (
        <>
          <XIcon className="size-3.5" aria-hidden="true" />
          Declined
        </>
      )}
    </p>
  )
}

/** Wires a "connect" prompt to the connector types, agent names and the connect request. */
function ConnectPrompt({
  message,
  prompt,
  isLatest,
  disabled,
  onDecline,
  onDismiss,
}: {
  message: Message
  prompt: Prompt
  isLatest: boolean
  disabled: boolean
  onDecline: () => void
  onDismiss: () => void
}) {
  const types = useConnectorTypes()
  const { data: agents } = useQuery(agentsQueryOptions)
  const connect = useConnectPrompt(message)
  const agentNames = new Map((agents ?? []).map((a) => [a.id, a.name]))
  return (
    <ConnectCard
      prompt={prompt}
      messageId={message.id}
      type={types.data?.find((t) => t.type === prompt.connection?.type)}
      typesLoading={types.isPending}
      agentNames={agentNames}
      isLatest={isLatest}
      disabled={disabled}
      onConnect={connect}
      onDecline={onDecline}
      onDismiss={onDismiss}
    />
  )
}

function SecretPrompt({
  message,
  prompt,
  isLatest,
  disabled,
  onDecline,
  onDismiss,
}: {
  message: Message
  prompt: Prompt
  isLatest: boolean
  disabled: boolean
  onDecline: () => void
  onDismiss: () => void
}) {
  const { data: agents } = useQuery(agentsQueryOptions)
  const provide = useProvideSecret(message)
  const agentName = agents?.find((a) => a.id === message.author.agentId)?.name ?? "The agent"
  return (
    <SecretCard
      prompt={prompt}
      agentName={agentName}
      isLatest={isLatest}
      disabled={disabled}
      onSave={provide}
      onDecline={onDecline}
      onDismiss={onDismiss}
    />
  )
}

/**
 * An agent's question with clickable answers. Pending: options (single answers on click, multi
 * toggles then submits), optional "type your own", or a text field. Answered: collapses to the
 * chosen answers, dimmed. Dismissed: just the question, muted. The card is the record of the
 * answer; no separate user message is shown.
 */
export function PromptCard({
  message,
  prompt,
  isLatest,
}: {
  message: Message
  prompt: Prompt
  /** The latest message in the chat: enables dismiss and letter-key shortcuts. */
  isLatest: boolean
}) {
  const answer = useAnswerPrompt(message)
  const dismiss = useDismissPrompt(message)
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  // A 409 means it was settled elsewhere; the refetch shows the real state, so no error.
  const rawError = answer.error ?? dismiss.error
  const error = rawError instanceof ApiError && rawError.status === 409 ? null : rawError
  const busy = answer.isPending || dismiss.isPending

  function onAnswer(value: PromptAnswer) {
    answer.mutate(value, {
      onSuccess: () => {
        const chatId = chatToOpen(prompt, value)
        if (!chatId) return
        void ensureChatLoaded(queryClient, chatId).then(() =>
          navigate({ to: "/chats/$chatId", params: { chatId } }),
        )
      },
    })
  }

  const dismissed = prompt.status === "dismissed"

  if (prompt.kind === "approval" && prompt.preview) {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <ActionPreviewCard
          prompt={prompt}
          preview={prompt.preview}
          isLatest={isLatest}
          disabled={busy}
          onAnswer={onAnswer}
          onDismiss={() => dismiss.mutate()}
        />
        {error && (
          <p role="alert" className="px-1.5 text-xs text-destructive">
            {error.message}
          </p>
        )}
      </div>
    )
  }

  if (prompt.kind === "secret") {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <SecretPrompt
          message={message}
          prompt={prompt}
          isLatest={isLatest}
          disabled={busy}
          onDecline={() => onAnswer(DECLINE_SECRET)}
          onDismiss={() => dismiss.mutate()}
        />
        {error && (
          <p role="alert" className="px-1.5 text-xs text-destructive">
            {error.message}
          </p>
        )}
      </div>
    )
  }

  if (prompt.kind === "connect") {
    return (
      <div className="flex min-w-0 flex-col gap-1.5">
        <ConnectPrompt
          message={message}
          prompt={prompt}
          isLatest={isLatest}
          disabled={busy}
          onDecline={() => onAnswer(DECLINE_CONNECT)}
          onDismiss={() => dismiss.mutate()}
        />
        {error && (
          <p role="alert" className="px-1.5 text-xs text-destructive">
            {error.message}
          </p>
        )}
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <div className="flex items-start gap-2">
        <p
          className={cn(
            "min-w-0 flex-1 px-1.5 pt-1 font-medium wrap-break-word whitespace-pre-line",
            dismissed && "font-normal text-muted-foreground",
          )}
        >
          {prompt.question}
        </p>
        {prompt.status === "pending" && isLatest && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Dismiss question"
            className="text-muted-foreground"
            disabled={busy}
            onClick={() => dismiss.mutate()}
          >
            <XIcon />
          </Button>
        )}
      </div>
      {prompt.kind === "approval" && prompt.status === "pending" && (
        <ApprovalActions
          onAnswer={onAnswer}
          disabled={busy}
          alwaysAllow={offersAlwaysAllow(prompt)}
        />
      )}
      {prompt.kind === "approval" && <ApprovalOutcome prompt={prompt} />}
      {prompt.kind !== "approval" && prompt.status === "pending" && (
        <PendingCard
          message={message}
          prompt={prompt}
          isLatest={isLatest}
          onAnswer={onAnswer}
          disabled={busy}
        />
      )}
      {prompt.kind !== "approval" && prompt.status === "answered" && (
        <AnsweredCard prompt={prompt} />
      )}
      {dismissed && <p className="px-1.5 pb-1 text-xs text-muted-foreground">Dismissed</p>}
      {error && (
        <p role="alert" className="px-1.5 text-xs text-destructive">
          {error.message}
        </p>
      )}
    </div>
  )
}

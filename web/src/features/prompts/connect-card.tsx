import { Link } from "@tanstack/react-router"
import { cn } from "cn"
import { CheckIcon, ChevronRightIcon, XIcon } from "lucide-react"
import { useEffect, useRef, useState, type FormEvent } from "react"

import { Button } from "@/components/ui/button"
import { FieldGroup } from "@/components/ui/field"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import {
  ConnectorFields,
  ConnectorIcon,
  initialValues,
  SetupSteps,
  validateForm,
  type ConnectorType,
} from "@/features/connectors"
import { ApiError } from "@/lib/api-client"

import {
  accessLabel,
  configRows,
  connectFields,
  connectOutcome,
  connectRequestBody,
  type ConnectPromptRequest,
  type PromptConnection,
} from "./connect"
import type { Prompt } from "./logic"

/** Nested radius: the bubble (radius-xl) pads the card by 0.5rem. */
const INNER_RADIUS = "rounded-[calc(var(--radius-xl)-0.5rem)]"

function ConnectForm({
  connection,
  type,
  agentNames,
  disabled,
  onConnect,
  onDecline,
}: {
  connection: PromptConnection
  type: ConnectorType
  agentNames: Map<string, string>
  disabled: boolean
  onConnect: (body: ConnectPromptRequest) => Promise<void>
  onDecline: () => void
}) {
  const [fields] = useState(() => connectFields(type))
  const [values, setValues] = useState(() => initialValues(fields))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const errorRef = useRef<HTMLParagraphElement>(null)
  const rows = configRows(connection, type)

  // The card is usually the last thing in the chat, so a new error can land below the fold.
  useEffect(() => {
    if (error) errorRef.current?.scrollIntoView?.({ block: "nearest", behavior: "smooth" })
  }, [error])

  async function submit(event: FormEvent) {
    event.preventDefault()
    const next = validateForm(fields, values)
    setErrors(next)
    if (Object.keys(next).length > 0) return
    setPending(true)
    setError(null)
    try {
      await onConnect(connectRequestBody(fields, values))
      // Don't keep secrets around once they're saved.
      setValues(initialValues(fields))
    } catch (e) {
      // 400: the app rejected the details; 502: barn couldn't reach it. The prompt stays
      // pending so the user can fix the values and try again.
      setError(
        e instanceof ApiError || e instanceof Error ? e.message : "Couldn't connect. Try again.",
      )
    } finally {
      setPending(false)
    }
  }

  const busy = disabled || pending
  return (
    <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-3 px-1.5 pb-1">
      {rows.length > 0 && (
        <dl className={cn("flex flex-col gap-1.5 bg-background/60 p-2 text-xs", INNER_RADIUS)}>
          {rows.map((row) => (
            <div key={row.key} className="flex min-w-0 flex-col gap-0.5">
              <dt className="text-muted-foreground">{row.label}</dt>
              <dd className="font-mono break-all">{row.value}</dd>
            </div>
          ))}
        </dl>
      )}
      <p className="text-xs text-muted-foreground">
        {accessLabel(connection.agentIds, agentNames)}
      </p>
      {type.setup.steps.length > 0 && <SetupGuide type={type} defaultOpen={fields.length > 0} />}
      {fields.length > 0 && (
        <FieldGroup className="gap-4">
          <ConnectorFields
            idPrefix={`connect-${connection.type}`}
            fields={fields}
            values={values}
            errors={errors}
            disabled={busy}
            onChange={(id, value) => {
              setValues((v) => ({ ...v, [id]: value }))
              setErrors((e) => ({ ...e, [id]: "" }))
              setError(null)
            }}
          />
        </FieldGroup>
      )}
      {error && (
        <p ref={errorRef} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button type="button" size="sm" variant="secondary" disabled={busy} onClick={onDecline}>
          Decline
        </Button>
        <Button type="submit" size="sm" disabled={busy}>
          {pending && <Spinner />}
          {pending ? `Checking with ${type.name}…` : "Connect"}
        </Button>
      </div>
    </form>
  )
}

/** The first line is the question; the rest is the agent's reason, in regular weight. */
function QuestionText({ question, muted }: { question: string; muted: boolean }) {
  const newline = question.indexOf("\n")
  const title = newline === -1 ? question : question.slice(0, newline)
  const reason = newline === -1 ? "" : question.slice(newline + 1).trim()
  return (
    <div
      className={cn(
        "flex flex-col gap-0.5 px-1.5 wrap-break-word",
        muted && "text-muted-foreground",
      )}
    >
      <p className={cn(!muted && "font-medium")}>{title}</p>
      {reason && <p className="whitespace-pre-line">{reason}</p>}
    </div>
  )
}

/** "How to get these": the type's setup steps, collapsible to keep the card compact. */
function SetupGuide({ type, defaultOpen }: { type: ConnectorType; defaultOpen: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  const id = `connect-guide-${type.type}`
  return (
    <div className={cn("flex flex-col gap-2 border bg-background/40 p-2", INNER_RADIUS)}>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        className="flex items-center gap-1.5 text-left text-xs font-medium outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50"
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronRightIcon
          aria-hidden="true"
          className={cn("size-3.5 text-muted-foreground", open && "rotate-90")}
        />
        How to get these
      </button>
      {open && (
        <div id={id}>
          <SetupSteps steps={type.setup.steps} label={`How to connect ${type.name}`} />
        </div>
      )}
    </div>
  )
}

function ConnectOutcome({ prompt }: { prompt: Prompt }) {
  const outcome = connectOutcome(prompt)
  const accountId = prompt.connection?.accountId
  if (outcome === "connected" && accountId) {
    return (
      <p className="flex items-center gap-1.5 px-1.5 pb-1 text-xs font-medium">
        <CheckIcon className="size-3.5" aria-hidden="true" />
        Connected
        <span aria-hidden="true" className="text-muted-foreground">
          ·
        </span>
        <Link
          to="/settings"
          search={{ connector: accountId }}
          className="font-normal text-muted-foreground underline underline-offset-4 hover:text-foreground"
        >
          View connection
        </Link>
      </p>
    )
  }
  if (outcome === "declined") {
    return (
      <p className="flex items-center gap-1.5 px-1.5 pb-1 text-xs font-medium text-muted-foreground">
        <XIcon className="size-3.5" aria-hidden="true" />
        Declined
      </p>
    )
  }
  return null
}

/**
 * An agent proposing to connect an app: what it is, the settings the agent filled in, who
 * gets access, and inputs for any secrets. Secrets never leave this card's state except in
 * the connect request itself.
 */
export function ConnectCard({
  prompt,
  type,
  typesLoading,
  agentNames,
  isLatest,
  disabled,
  onConnect,
  onDecline,
  onDismiss,
}: {
  prompt: Prompt
  /** The proposed connector type, from `/connectors/types` (undefined while loading). */
  type: ConnectorType | undefined
  typesLoading: boolean
  agentNames: Map<string, string>
  isLatest: boolean
  disabled: boolean
  onConnect: (body: ConnectPromptRequest) => Promise<void>
  onDecline: () => void
  onDismiss: () => void
}) {
  const connection = prompt.connection
  const dismissed = prompt.status === "dismissed"
  const typeName = type?.name ?? connection?.type ?? "app"

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex items-center gap-2.5 px-1.5 pt-1">
        <ConnectorIcon type={connection?.type ?? ""} className="size-7" />
        <p className="min-w-0 flex-1 truncate text-sm">
          <span className="font-medium">{connection?.name ?? typeName}</span>
          <span className="text-muted-foreground"> · {typeName}</span>
        </p>
        {prompt.status === "pending" && isLatest && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Dismiss question"
            className="text-muted-foreground"
            disabled={disabled}
            onClick={onDismiss}
          >
            <XIcon />
          </Button>
        )}
      </div>
      <QuestionText question={prompt.question} muted={prompt.status !== "pending"} />
      {prompt.status === "pending" &&
        connection &&
        (type ? (
          <ConnectForm
            connection={connection}
            type={type}
            agentNames={agentNames}
            disabled={disabled}
            onConnect={onConnect}
            onDecline={onDecline}
          />
        ) : typesLoading ? (
          <div className="flex flex-col gap-2 px-1.5 pb-1" aria-hidden="true">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="ml-auto h-8 w-40" />
          </div>
        ) : (
          <div className="flex items-center justify-between gap-2 px-1.5 pb-1">
            <p className="text-xs text-destructive">barn doesn't know how to connect {typeName}.</p>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={disabled}
              onClick={onDecline}
            >
              Decline
            </Button>
          </div>
        ))}
      {prompt.status === "answered" && <ConnectOutcome prompt={prompt} />}
      {dismissed && <p className="px-1.5 pb-1 text-xs text-muted-foreground">Dismissed</p>}
    </div>
  )
}

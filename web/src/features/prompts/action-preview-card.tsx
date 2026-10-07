import { cn } from "cn"
import { CheckIcon, XIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { ConnectorIcon } from "@/features/connectors"

import { approvalAnswer, type Prompt, type PromptAnswer } from "./logic"
import {
  bodyText,
  isLongText,
  previewStatus,
  toPreviewNode,
  type ActionPreview,
  type PreviewNode,
  type PreviewRow,
} from "./preview"

/*
 * Radii follow the nested rule: the bubble (radius-xl) pads the card by 0.5rem, so the panel
 * gets radius-xl - 0.5rem; the panel pads nested blocks by 0.625rem, so they get what's left.
 */
const PANEL_RADIUS = "rounded-[calc(var(--radius-xl)-0.5rem)]"
const BLOCK_RADIUS = "rounded-[max(4px,calc(var(--radius-xl)-1.125rem))]"

/** Long text, clamped to a dozen lines with a "Show more" toggle. */
function Collapsible({ text, className }: { text: string; className?: string }) {
  const [expanded, setExpanded] = useState(false)
  const long = isLongText(text)
  return (
    <div className="flex min-w-0 flex-col items-start gap-1">
      <div className={cn(className, long && !expanded && "line-clamp-12")}>{text}</div>
      {long && (
        <button
          type="button"
          className="text-xs text-muted-foreground underline underline-offset-4 select-none hover:text-foreground"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "Show less" : "Show more"}
        </button>
      )}
    </div>
  )
}

function NodeView({ node }: { node: PreviewNode }) {
  switch (node.kind) {
    case "empty":
      return <span className="text-muted-foreground">{node.text}</span>
    case "text":
      return (
        <Collapsible
          text={node.text}
          className={cn(
            "wrap-break-word",
            node.multiline && "whitespace-pre-line",
            node.mono && "font-mono text-xs break-all",
          )}
        />
      )
    case "json":
      return <Collapsible text={node.text} className="font-mono text-xs break-all" />
    case "chips":
      return (
        <ul className="flex flex-wrap gap-1">
          {node.items.map((item, i) => (
            // oxlint-disable-next-line react/no-array-index-key -- values can repeat
            <li key={i} className="rounded-full border bg-background px-2 py-0.5 text-xs">
              {item}
            </li>
          ))}
        </ul>
      )
    case "rows":
      return <Rows rows={node.rows} nested />
    default:
      // "blocks": one bordered block of rows per item.
      return (
        <div className="flex flex-col gap-1.5">
          {node.items.map((rows, i) => (
            // oxlint-disable-next-line react/no-array-index-key -- items are positional
            <div key={i} className={cn("border bg-background/60 p-2", BLOCK_RADIUS)}>
              <Rows rows={rows} nested />
            </div>
          ))}
        </div>
      )
  }
}

/** Label/value rows: muted, narrow labels on the left, values to the right. */
function Rows({ rows, nested = false }: { rows: PreviewRow[]; nested?: boolean }) {
  return (
    <dl
      className={cn(
        "grid min-w-0 gap-x-3 gap-y-1.5",
        nested
          ? "grid-cols-[minmax(0,6rem)_minmax(0,1fr)] text-xs"
          : "grid-cols-[minmax(0,7rem)_minmax(0,1fr)]",
      )}
    >
      {rows.map((row) => (
        <div key={row.key} className="contents">
          <dt className="truncate text-muted-foreground" title={row.label}>
            {row.label}
          </dt>
          <dd className="min-w-0">
            <NodeView node={row.node} />
          </dd>
        </div>
      ))}
    </dl>
  )
}

function StatusChip({ prompt }: { prompt: Prompt }) {
  const status = previewStatus(prompt)
  let mark: ReactNode
  if (status.tone === "warning") {
    mark = <span className="size-1.5 rounded-full bg-warning" aria-hidden="true" />
  } else if (status.tone === "done") {
    mark = <CheckIcon className="size-3" aria-hidden="true" />
  }
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs whitespace-nowrap select-none",
        status.tone === "muted" ? "text-muted-foreground" : "font-medium",
      )}
    >
      {mark}
      {status.label}
    </span>
  )
}

/**
 * An approval as a structured preview of the action: what it is and where, the arguments as
 * readable rows, the main content large, then Approve (labelled with the action's verb) and
 * Decline. No keyboard shortcuts, so nothing is approved by accident.
 */
export function ActionPreviewCard({
  prompt,
  preview,
  isLatest,
  disabled,
  onAnswer,
  onDismiss,
}: {
  prompt: Prompt
  preview: ActionPreview
  isLatest: boolean
  disabled: boolean
  onAnswer: (answer: PromptAnswer) => void
  onDismiss: () => void
}) {
  const pending = prompt.status === "pending"
  const rows: PreviewRow[] = preview.fields.map((field) => ({
    key: field.key,
    label: field.label,
    node: toPreviewNode(field.value),
  }))
  const hasPanel = rows.length > 0 || preview.body !== null

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex items-center gap-2.5 px-1.5 pt-1">
        <ConnectorIcon type={preview.appType ?? "barn"} className="size-7" />
        <p className="min-w-0 flex-1 truncate text-sm">
          <span className="font-medium">{preview.title}</span>
          {preview.appName && <span className="text-muted-foreground"> · {preview.appName}</span>}
        </p>
        <StatusChip prompt={prompt} />
        {pending && isLatest && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label="Dismiss request"
            className="-mr-1 text-muted-foreground"
            disabled={disabled}
            onClick={onDismiss}
          >
            <XIcon />
          </Button>
        )}
      </div>
      {hasPanel && (
        <div
          className={cn(
            "flex min-w-0 flex-col gap-3 border bg-background/50 p-2.5 text-sm",
            PANEL_RADIUS,
            !pending && "opacity-75",
          )}
        >
          {rows.length > 0 && <Rows rows={rows} />}
          {preview.body && (
            <div
              className={cn("flex min-w-0 flex-col gap-1", rows.length > 0 && "border-t pt-3")}
              aria-label={preview.body.label}
              role="group"
            >
              <Collapsible
                text={bodyText(preview.body)}
                className="text-[0.95rem] leading-relaxed wrap-break-word whitespace-pre-line"
              />
            </div>
          )}
        </div>
      )}
      {preview.note && <p className="px-1.5 text-xs text-muted-foreground">{preview.note}</p>}
      {pending && (
        <div className="flex flex-wrap gap-2 px-0.5 pb-0.5">
          <Button size="sm" disabled={disabled} onClick={() => onAnswer(approvalAnswer(true))}>
            {preview.verb}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={disabled}
            onClick={() => onAnswer(approvalAnswer(false))}
          >
            Decline
          </Button>
        </div>
      )}
    </div>
  )
}

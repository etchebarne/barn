import { cn } from "cn"
import { ExternalLinkIcon } from "lucide-react"

import { CopyTextButton } from "@/components/copy-button"
import { buttonVariants } from "@/components/ui/button"

import type { SetupStep } from "./logic"

/**
 * A short numbered guide ("How to connect", "Receive events"). Links open in a new tab; copy
 * actions copy long text (like an app manifest) without showing it.
 */
export function SetupSteps({
  steps,
  label,
  className,
}: {
  steps: SetupStep[]
  /** Accessible name for the list. */
  label: string
  className?: string
}) {
  if (steps.length === 0) return null
  return (
    <ol aria-label={label} className={cn("flex flex-col gap-3", className)}>
      {steps.map((step, i) => (
        // oxlint-disable-next-line react/no-array-index-key -- steps are positional
        <li key={i} className="flex gap-2.5 text-sm">
          <span
            aria-hidden="true"
            className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium tabular-nums select-none"
          >
            {i + 1}
          </span>
          <div className="flex min-w-0 flex-1 flex-col gap-1.5 pt-px">
            <p className="leading-snug wrap-break-word">{step.text}</p>
            {(step.link || step.copy) && (
              <div className="flex flex-wrap items-center gap-2">
                {step.link && (
                  // A real link (styled as a button) so it opens in a new tab and reads as a link.
                  <a
                    href={step.link.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className={buttonVariants({ variant: "outline", size: "xs" })}
                  >
                    {step.link.label}
                    <ExternalLinkIcon aria-hidden="true" />
                  </a>
                )}
                {step.copy && (
                  <span className="flex items-center gap-1.5">
                    <CopyTextButton text={step.copy.text} label={step.copy.label} />
                    <span className="text-xs text-muted-foreground">{step.copy.label}</span>
                  </span>
                )}
              </div>
            )}
          </div>
        </li>
      ))}
    </ol>
  )
}

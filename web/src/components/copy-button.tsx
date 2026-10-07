import { CheckIcon, CopyIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/** Copies `text`, with a short-lived `copied` flag for feedback. Shared by every copy button. */
export function useCopy(text: string) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])

  async function copy() {
    try {
      await writeClipboard(text)
      setCopied(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => setCopied(false), 1500)
    } catch {
      // Copying was refused; nothing sensible to do.
    }
  }

  return { copied, copy }
}

/** A small secondary "Copy" button with a text label, for things too long to show (manifests). */
export function CopyTextButton({ text, label }: { text: string; label: string }) {
  const { copied, copy } = useCopy(text)
  return (
    <Button
      type="button"
      variant="secondary"
      size="xs"
      aria-label={copied ? `Copied ${label}` : `Copy ${label}`}
      onClick={() => void copy()}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
      {copied ? "Copied" : "Copy"}
    </Button>
  )
}

export function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const { copied, copy } = useCopy(text)

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={copied ? "Copied" : label}
            onClick={() => void copy()}
            className="text-muted-foreground"
          />
        }
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </TooltipTrigger>
      <TooltipContent>{copied ? "Copied" : label}</TooltipContent>
    </Tooltip>
  )
}

/**
 * The Clipboard API only exists in secure contexts (HTTPS or localhost). Over plain http, fall
 * back to selecting a hidden textarea and the legacy copy command.
 */
async function writeClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) return navigator.clipboard.writeText(text)
  const area = document.createElement("textarea")
  area.value = text
  area.setAttribute("readonly", "")
  area.style.position = "fixed"
  area.style.opacity = "0"
  document.body.append(area)
  area.select()
  const ok = document.execCommand("copy")
  area.remove()
  if (!ok) throw new Error("copy failed")
}

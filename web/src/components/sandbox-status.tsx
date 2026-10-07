import { cn } from "cn"

import { sandboxStatusLabel, type SandboxStatus } from "@/lib/sandbox"

/** The computer's state as a dot and a word. */
export function SandboxStatusText({
  status,
  className,
}: {
  status: SandboxStatus | "restarting"
  className?: string
}) {
  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <span
        aria-hidden="true"
        className={cn(
          "size-1.5 shrink-0 rounded-full",
          status === "running"
            ? "bg-emerald-500"
            : status === "restarting"
              ? "bg-amber-500"
              : "bg-muted-foreground/50",
        )}
      />
      {sandboxStatusLabel(status)}
    </span>
  )
}

import { cn } from "cn"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"

export function initials(name: string): string {
  const words = name
    .trim()
    .split(/[\s_-]+/)
    .filter(Boolean)
  if (words.length === 0) return "?"
  if (words.length === 1) return (words[0] ?? "").slice(0, 2).toUpperCase()
  return ((words[0]?.[0] ?? "") + (words[1]?.[0] ?? "")).toUpperCase()
}

/** The one avatar used for agents everywhere (sidebar, headers, messages). */
export function AgentAvatar({
  name,
  size = "default",
  className,
}: {
  name: string
  size?: "default" | "sm" | "lg"
  className?: string
}) {
  return (
    <Avatar size={size} className={className} aria-hidden="true">
      <AvatarFallback className={cn("font-medium")}>{initials(name)}</AvatarFallback>
    </Avatar>
  )
}

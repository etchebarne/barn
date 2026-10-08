import { cn } from "cn"
import type * as React from "react"

import { Aura } from "./aura"

export function initials(name: string): string {
  const words = name
    .trim()
    .split(/[\s_-]+/)
    .filter(Boolean)
  if (words.length === 0) return "?"
  if (words.length === 1) return (words[0] ?? "").slice(0, 2).toUpperCase()
  return ((words[0]?.[0] ?? "") + (words[1]?.[0] ?? "")).toUpperCase()
}

type Size = "default" | "sm" | "md" | "lg"

const pixels: Record<Size, number> = { sm: 24, default: 32, md: 36, lg: 40 }

const frame: Record<Size, string> = {
  sm: "size-6",
  default: "size-8",
  md: "size-9",
  lg: "size-10",
}

/**
 * The one avatar used for agents everywhere (sidebar, headers, messages), seeded by the agent's
 * id so it survives renames. Falls back to initials when the id isn't known yet. The aura drifts
 * slowly on its own; `active` (the agent is working) makes it a little livelier.
 */
export function AgentAvatar({
  id,
  name,
  size = "default",
  active = false,
  className,
}: {
  id?: string
  name: string
  size?: Size
  active?: boolean
  className?: string
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center overflow-hidden rounded-full select-none",
        !id && "bg-muted font-medium text-muted-foreground",
        !id && (size === "sm" ? "text-[10px]" : "text-xs"),
        frame[size],
        className,
      )}
    >
      {id ? <Aura seed={id} active={active} /> : initials(name)}
    </span>
  )
}

/**
 * Avatar for group chats: the members' auras overlapping, up to three, each cut out of the one
 * in front with a ring in the background color.
 */
export function GroupAvatar({
  memberIds,
  size = "default",
  className,
}: {
  memberIds: string[]
  size?: Size
  className?: string
}) {
  const shown = memberIds.slice(0, 3)
  if (shown.length <= 1) {
    return <AgentAvatar id={shown[0]} name="?" size={size} className={className} />
  }
  // Each circle is 62% (two members) or 54% (three) of the frame, placed along the diagonal.
  const d = shown.length === 2 ? 0.62 : 0.54
  const px = pixels[size]
  const diameter = px * d
  const step = shown.length === 2 ? 1 - d : (1 - d) / 2
  const spots = shown.map((_, i) => ({
    x: step * i * px,
    y: (shown.length === 2 ? step * i : ([0, 0.46, 0.23][i] ?? 0)) * px,
  }))
  // A clear gap between overlapping circles: each one is cut out where the ones in front of it
  // sit (plus the gap), so whatever is behind the avatar shows through.
  const gap = size === "sm" ? 1.5 : 2
  const cutout = (i: number) => {
    const holes = spots.slice(i + 1).map((s) => {
      const cx = s.x - spots[i].x + diameter / 2
      const cy = s.y - spots[i].y + diameter / 2
      const r = diameter / 2 + gap
      return `radial-gradient(circle at ${cx}px ${cy}px, transparent ${r}px, #000 ${r + 0.5}px)`
    })
    if (holes.length === 0) return undefined
    const mask = holes.join(", ")
    return {
      maskImage: mask,
      WebkitMaskImage: mask,
      maskComposite: "intersect",
      WebkitMaskComposite: "source-in",
    } satisfies React.CSSProperties
  }
  return (
    <span
      aria-hidden="true"
      className={cn("relative shrink-0 select-none", frame[size], className)}
    >
      {shown.map((id, i) => (
        <span
          key={id}
          className="absolute overflow-hidden rounded-full"
          style={{
            width: diameter,
            height: diameter,
            left: spots[i].x,
            top: spots[i].y,
            ...cutout(i),
          }}
        >
          <Aura seed={id} />
        </span>
      ))}
    </span>
  )
}

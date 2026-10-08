import { cn } from "cn"
import {
  BugIcon,
  CreditCardIcon,
  GitPullRequestIcon,
  HashIcon,
  NotebookTextIcon,
  ShieldCheckIcon,
  type LucideIcon,
  PlugIcon,
  ServerIcon,
  SquareKanbanIcon,
  SquareTerminalIcon,
  WebhookIcon,
} from "lucide-react"

import { LogoGlyph } from "@/components/brand-mark"

import { BRAND_LOGOS } from "./brand-logos"

const ICONS: Record<string, LucideIcon | typeof LogoGlyph> = {
  slack: HashIcon,
  github: GitPullRequestIcon,
  linear: SquareKanbanIcon,
  render: ServerIcon,
  mcp_local: SquareTerminalIcon,
  webhook: WebhookIcon,
  // Sign-in apps from the catalog, by catalog id.
  notion: NotebookTextIcon,
  sentry: BugIcon,
  stripe: CreditCardIcon,
  // openbot's own actions (e.g. deleting an agent).
  openbot: LogoGlyph,
  "always-allow": ShieldCheckIcon,
}

const SIZES = {
  sm: "size-8 rounded-[9px] [&_svg]:size-4.5",
  md: "size-10 rounded-[11px] [&_svg]:size-5.5",
  lg: "size-12 rounded-[13px] [&_svg]:size-7",
} as const

/**
 * The one icon per connector type, used everywhere (grid, picker, detail, agent sheet): the
 * app's own logo on an app-icon tile when there is one, else a neutral tile with a symbol.
 */
export function ConnectorIcon({
  type,
  size = "sm",
  className,
}: {
  type: string
  size?: keyof typeof SIZES
  className?: string
}) {
  const brand = BRAND_LOGOS[type]
  if (brand) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          "flex shrink-0 items-center justify-center shadow-[inset_0_0_0_1px_rgb(0_0_0/8%)] dark:shadow-[inset_0_0_0_1px_rgb(255_255_255/10%)]",
          SIZES[size],
          className,
        )}
        style={{ background: brand.tile }}
      >
        <svg viewBox="0 0 24 24">{brand.mark}</svg>
      </span>
    )
  }
  const Icon = ICONS[type] ?? PlugIcon
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center border bg-muted text-foreground [&_svg]:stroke-[1.75]",
        SIZES[size],
        size === "lg" && "[&_svg]:size-6",
        className,
      )}
    >
      <Icon />
    </span>
  )
}

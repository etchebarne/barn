import { cn } from "cn"
import {
  BlocksIcon,
  BugIcon,
  CreditCardIcon,
  GitPullRequestIcon,
  HashIcon,
  HouseIcon,
  NotebookTextIcon,
  ShieldCheckIcon,
  type LucideIcon,
  PlugIcon,
  ServerIcon,
  SquareKanbanIcon,
  SquareTerminalIcon,
  WebhookIcon,
} from "lucide-react"

const ICONS: Record<string, LucideIcon> = {
  slack: HashIcon,
  github: GitPullRequestIcon,
  linear: SquareKanbanIcon,
  render: ServerIcon,
  mcp: BlocksIcon,
  mcp_local: SquareTerminalIcon,
  webhook: WebhookIcon,
  // Sign-in apps from the catalog, by catalog id.
  notion: NotebookTextIcon,
  sentry: BugIcon,
  stripe: CreditCardIcon,
  // openbot's own actions (e.g. archiving an agent).
  openbot: HouseIcon,
  "always-allow": ShieldCheckIcon,
}

/** The one icon per connector type, used everywhere (list, picker, detail, agent sheet). */
export function ConnectorIcon({ type, className }: { type: string; className?: string }) {
  const Icon = ICONS[type] ?? PlugIcon
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded-lg border bg-muted/50 text-foreground [&_svg]:size-4",
        className,
      )}
    >
      <Icon />
    </span>
  )
}

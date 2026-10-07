import { cn } from "cn"
import type { ReactNode } from "react"

import { SidebarTrigger } from "@/components/ui/sidebar"

/** Header bar for the main pane. Shows the sidebar trigger on mobile. */
export function PageHeader({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <header
      className={cn(
        "flex h-14 shrink-0 items-center gap-2 border-b bg-background px-3 md:px-4",
        className,
      )}
    >
      <SidebarTrigger className="-ml-1 md:hidden" />
      <div className="flex min-w-0 flex-1 items-center gap-3">{children}</div>
    </header>
  )
}

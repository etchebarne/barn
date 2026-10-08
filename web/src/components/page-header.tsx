import { cn } from "cn"
import { ChevronLeftIcon } from "lucide-react"
import type { ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { useIsMobile } from "@/hooks/use-mobile"
import { useScreenBack } from "@/lib/mobile-nav"

/** Back to the chat list (mobile pushed screens). */
function BackButton() {
  const back = useScreenBack()
  return (
    <Button
      variant="ghost"
      size="icon"
      className="-ml-2 shrink-0"
      aria-label="Back"
      // detail is 0 for keyboard-activated clicks: no slide for keyboard navigation.
      onClick={(event) => void back(event.detail > 0)}
    >
      <ChevronLeftIcon className="size-5" />
    </Button>
  )
}

/**
 * Header bar for a screen. On mobile, screens are pushed over the chat list, so it starts with
 * a back button and makes room for the status bar.
 */
export function PageHeader({
  children,
  actions,
  className,
}: {
  children: ReactNode
  /** Buttons on the right edge. */
  actions?: ReactNode
  className?: string
}) {
  const mobile = useIsMobile()
  return (
    <header
      className={cn(
        // No rule under it: the transcript fades out beneath it instead.
        "flex h-13 shrink-0 items-center gap-2 bg-background px-3 md:px-4",
        mobile && "h-[calc(3.5rem+env(safe-area-inset-top))] pt-[env(safe-area-inset-top)]",
        className,
      )}
    >
      {mobile && <BackButton />}
      <div className="flex min-w-0 flex-1 items-center gap-3">{children}</div>
      {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
    </header>
  )
}

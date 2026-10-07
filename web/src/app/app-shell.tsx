import { Outlet, useLocation } from "@tanstack/react-router"
import { useEffect, useLayoutEffect, useRef } from "react"

import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { AgentDetailsSheet } from "@/features/agents"
import { ChatSidebar, CommandPalette, MobileChatList } from "@/features/chats"
import { useIsMobile } from "@/hooks/use-mobile"
import {
  PushedScreenContext,
  shouldAnimatePush,
  slideIn,
  trackNavigationInput,
} from "@/lib/mobile-nav"

import { useRealtime } from "./realtime"
import { useSyncTimezone } from "./timezone"

/**
 * A screen pushed over the chat list on mobile (a chat, Settings, …). Slides in when opened by
 * a tap, not by keyboard or browser back/forward.
 */
function PushedScreen({ children }: { children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (shouldAnimatePush()) slideIn(ref.current)
  }, [])
  return (
    <PushedScreenContext value={ref}>
      <div ref={ref} className="absolute inset-0 z-10 flex flex-col bg-background shadow-xl">
        {children}
      </div>
    </PushedScreenContext>
  )
}

/**
 * Mobile: the chat list is the home screen (`/`), and everything else is pushed over it with
 * a back button. The list stays mounted underneath, so its scroll position survives.
 */
export function MobileShell() {
  const { pathname } = useLocation()
  const atList = pathname === "/"
  useEffect(() => trackNavigationInput(), [])
  return (
    <div className="relative h-svh w-full overflow-hidden bg-background">
      <div className="absolute inset-0" inert={!atList} aria-hidden={!atList}>
        <MobileChatList />
      </div>
      {!atList && (
        <PushedScreen key={pathname}>
          <Outlet />
        </PushedScreen>
      )}
    </div>
  )
}

/** Signed-in layout: chat sidebar on the left and the main pane (desktop), or screens (mobile). */
export function AppShell() {
  useRealtime()
  useSyncTimezone()
  const mobile = useIsMobile()
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      {mobile ? (
        <MobileShell />
      ) : (
        <>
          <ChatSidebar />
          <SidebarInset className="min-w-0">
            <Outlet />
          </SidebarInset>
        </>
      )}
      <AgentDetailsSheet />
      <CommandPalette />
    </SidebarProvider>
  )
}

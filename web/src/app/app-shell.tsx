import { Outlet } from "@tanstack/react-router"

import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { AgentDetailsSheet } from "@/features/agents"
import { ChatSidebar, CommandPalette } from "@/features/chats"

import { useRealtime } from "./realtime"
import { useSyncTimezone } from "./timezone"

/** Signed-in layout: chat sidebar on the left (a sheet on mobile), main pane on the right. */
export function AppShell() {
  useRealtime()
  useSyncTimezone()
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <ChatSidebar />
      <SidebarInset className="min-w-0">
        <Outlet />
      </SidebarInset>
      <AgentDetailsSheet />
      <CommandPalette />
    </SidebarProvider>
  )
}

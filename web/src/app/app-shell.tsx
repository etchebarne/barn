import { Outlet } from "@tanstack/react-router"

import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar"
import { AgentDetailsSheet } from "@/features/agents"
import { ChatSidebar } from "@/features/chats"

import { useRealtime } from "./realtime"

/** Signed-in layout: chat sidebar on the left (a sheet on mobile), main pane on the right. */
export function AppShell() {
  useRealtime()
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <ChatSidebar />
      <SidebarInset className="min-w-0">
        <Outlet />
      </SidebarInset>
      <AgentDetailsSheet />
    </SidebarProvider>
  )
}

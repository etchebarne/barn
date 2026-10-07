import { useQuery } from "@tanstack/react-query"
import { Link, useMatchRoute } from "@tanstack/react-router"
import { SettingsIcon } from "lucide-react"

import { BrandMark } from "@/components/brand-mark"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  useSidebar,
} from "@/components/ui/sidebar"
import { AgentAvatar, GroupAvatar, useAgentsById } from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"
import { useConnectionStore } from "@/lib/ws"

import { chatsQueryOptions } from "./api"
import { chatPreview, dmAgent } from "./preview"

function UnreadBadge({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <span
      className="ml-auto flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-medium text-primary-foreground tabular-nums"
      aria-label={`${count} unread`}
    >
      {count > 99 ? "99+" : count}
    </span>
  )
}

function ChatAvatar({ chat, agent }: { chat: Chat; agent: Agent | undefined }) {
  if (chat.kind === "dm") return <AgentAvatar id={agent?.id} name={agent?.name ?? chat.name} />
  return <GroupAvatar memberIds={chat.members.map((m) => m.agentId)} />
}

function ChatListItem({
  chat,
  agents,
  active,
  onNavigate,
}: {
  chat: Chat
  agents: Map<string, Agent>
  active: boolean
  onNavigate: () => void
}) {
  const agent = dmAgent(chat, agents)
  const unread = chat.unreadCount > 0
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        size="lg"
        isActive={active}
        className="h-auto gap-3 py-2 select-none"
        render={<Link to="/chats/$chatId" params={{ chatId: chat.id }} onClick={onNavigate} />}
      >
        <ChatAvatar chat={chat} agent={agent} />
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{chat.name}</span>
          </div>
          <span
            className={
              unread
                ? "truncate text-xs text-sidebar-foreground"
                : "truncate text-xs text-muted-foreground"
            }
          >
            {chatPreview(chat, agents)}
          </span>
        </div>
        <UnreadBadge count={chat.unreadCount} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function ConnectionNotice() {
  const status = useConnectionStore((s) => s.status)
  if (status !== "reconnecting") return null
  return (
    <p role="status" className="flex items-center gap-2 px-2 text-xs text-muted-foreground">
      <span className="size-1.5 rounded-full bg-amber-500" aria-hidden="true" />
      Reconnecting…
    </p>
  )
}

/** Left sidebar: chats (most recently active first), settings pinned at the bottom. */
export function ChatSidebar() {
  const { data: chats, isPending, error } = useQuery(chatsQueryOptions)
  const agents = useAgentsById()
  const matchRoute = useMatchRoute()
  const { isMobile, setOpenMobile } = useSidebar()
  const closeOnMobile = () => {
    if (isMobile) setOpenMobile(false)
  }
  const settingsActive = !!matchRoute({ to: "/settings" })

  return (
    <Sidebar collapsible={isMobile ? "offcanvas" : "none"} className="h-svh border-r">
      <SidebarHeader className="h-14 justify-center border-b px-4">
        <BrandMark />
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu aria-label="Chats" className="gap-0.5">
              {isPending &&
                Array.from({ length: 3 }, (_, i) => (
                  <SidebarMenuItem key={i}>
                    <SidebarMenuSkeleton showIcon className="h-12" />
                  </SidebarMenuItem>
                ))}
              {error && (
                <p className="px-2 py-1 text-sm text-destructive">
                  Couldn't load chats: {error.message}
                </p>
              )}
              {chats?.map((chat) => (
                <ChatListItem
                  key={chat.id}
                  chat={chat}
                  agents={agents}
                  active={!!matchRoute({ to: "/chats/$chatId", params: { chatId: chat.id } })}
                  onNavigate={closeOnMobile}
                />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="border-t">
        <ConnectionNotice />
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              isActive={settingsActive}
              className="select-none"
              render={<Link to="/settings" onClick={closeOnMobile} />}
            >
              <SettingsIcon />
              <span>Settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}

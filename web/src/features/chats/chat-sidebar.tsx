import { useQuery } from "@tanstack/react-query"
import { useMatchRoute } from "@tanstack/react-router"
import { cn } from "cn"

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuSkeleton,
} from "@/components/ui/sidebar"
import { useAgentsById } from "@/features/agents"
import { AccountMenu } from "@/features/auth"
import { useConnectionStore } from "@/lib/ws"

import { chatsQueryOptions } from "./api"
import { SearchButton } from "./command-palette"
import { useCategories } from "./sidebar-api"
import { NewCategory, SidebarSections } from "./sidebar-sections"

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

/**
 * The chat list: search on top, chats in the user's categories and order, account at the
 * bottom. The desktop sidebar and the mobile home screen both show it.
 */
function ChatListContent({ mobile }: { mobile: boolean }) {
  const { data: chats, isPending, error } = useQuery(chatsQueryOptions)
  const { data: categories, isPending: categoriesPending } = useCategories()
  const agents = useAgentsById()
  const matchRoute = useMatchRoute()

  return (
    <>
      {/* Top and bottom use the same button shape so the list's edges balance. */}
      <SidebarHeader
        className={cn(
          "justify-center border-b px-2",
          mobile
            ? "h-[calc(3.5rem+env(safe-area-inset-top))] pt-[env(safe-area-inset-top)]"
            : "h-14",
        )}
      >
        <SearchButton />
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu aria-label="Chats" className="gap-0.5">
              {(isPending || categoriesPending) &&
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
            </SidebarMenu>
            {chats && categories && (
              <SidebarSections
                chats={chats}
                categories={categories}
                agents={agents}
                // On mobile the list is its own screen; no row is "current" there.
                isActive={(chatId) =>
                  !mobile && !!matchRoute({ to: "/chats/$chatId", params: { chatId } })
                }
                onNavigate={() => {}}
              />
            )}
            {chats && categories && (
              <div className="mt-3">
                <NewCategory />
              </div>
            )}
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter
        className={cn("border-t", mobile && "pb-[max(0.5rem,env(safe-area-inset-bottom))]")}
      >
        <ConnectionNotice />
        <AccountMenu />
      </SidebarFooter>
    </>
  )
}

/** Desktop: the chat list as a fixed left sidebar. */
export function ChatSidebar() {
  return (
    <Sidebar collapsible="none" className="h-svh border-r">
      <ChatListContent mobile={false} />
    </Sidebar>
  )
}

/** Mobile: the chat list as the home screen. */
export function MobileChatList() {
  return (
    <div className="flex h-full flex-col bg-sidebar text-sidebar-foreground">
      <ChatListContent mobile />
    </div>
  )
}

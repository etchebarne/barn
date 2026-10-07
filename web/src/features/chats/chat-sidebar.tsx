import { useQuery } from "@tanstack/react-query"
import { useMatchRoute } from "@tanstack/react-router"

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
  useSidebar,
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

/** Left sidebar: search on top, chats in the user's categories and order, account at the bottom. */
export function ChatSidebar() {
  const { data: chats, isPending, error } = useQuery(chatsQueryOptions)
  const { data: categories, isPending: categoriesPending } = useCategories()
  const agents = useAgentsById()
  const matchRoute = useMatchRoute()
  const { isMobile, setOpenMobile } = useSidebar()
  const closeOnMobile = () => {
    if (isMobile) setOpenMobile(false)
  }

  return (
    <Sidebar collapsible={isMobile ? "offcanvas" : "none"} className="h-svh border-r">
      {/* Top and bottom use the same button shape so the sidebar's edges balance. */}
      <SidebarHeader className="h-14 justify-center border-b px-2">
        {/* On mobile the sidebar is a sheet: close it first so the palette never stacks on it. */}
        <SearchButton onOpen={closeOnMobile} />
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
                isActive={(chatId) => !!matchRoute({ to: "/chats/$chatId", params: { chatId } })}
                onNavigate={closeOnMobile}
              />
            )}
            {chats && categories && (
              <div className="mt-2">
                <NewCategory />
              </div>
            )}
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="border-t">
        <ConnectionNotice />
        <AccountMenu onNavigate={closeOnMobile} />
      </SidebarFooter>
    </Sidebar>
  )
}

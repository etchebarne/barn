import { useQuery } from "@tanstack/react-query"
import { Link, useMatchRoute } from "@tanstack/react-router"
import { cn } from "cn"
import {
  BotIcon,
  CalendarClockIcon,
  PlugIcon,
  SettingsIcon,
  SparklesIcon,
  SquarePenIcon,
  UsersIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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
} from "@/components/ui/sidebar"
import { useAgentsById } from "@/features/agents"
import { AccountMenu } from "@/features/auth"
import { useConnectionStore } from "@/lib/ws"

import { chatsQueryOptions } from "./api"
import { SearchButton } from "./command-palette"
import { useStartNew, type NewKind } from "./new-chat"
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

/** "New agent" / "New group" from the sidebar's ✎ button. */
function NewButton({ onNavigate }: { onNavigate?: () => void }) {
  const { creator, start } = useStartNew()
  if (!start) return null
  function go(kind: NewKind) {
    onNavigate?.()
    start?.(kind)
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="size-9 shrink-0 text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
            aria-label="New agent or group"
          />
        }
      >
        <SquarePenIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuItem onClick={() => go("agent")}>
          <BotIcon />
          <span className="flex flex-col">
            New agent
            <span className="text-xs text-muted-foreground">
              Ask {creator?.name ?? "an agent"} to make one
            </span>
          </span>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => go("group")}>
          <UsersIcon />
          <span className="flex flex-col">
            New group
            <span className="text-xs text-muted-foreground">Agents working together</span>
          </span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** The app's pages, always one click away above the account button. */
function SidebarNav({ mobile }: { mobile: boolean }) {
  const matchRoute = useMatchRoute()
  const links = [
    { to: "/schedule", label: "Schedule", icon: CalendarClockIcon },
    { to: "/skills", label: "Skills", icon: SparklesIcon },
    { to: "/connectors", label: "Connectors", icon: PlugIcon },
    { to: "/settings", label: "Settings", icon: SettingsIcon },
  ] as const
  return (
    <SidebarMenu className="gap-0.5">
      {links.map(({ to, label, icon: Icon }) => (
        <SidebarMenuItem key={to}>
          <SidebarMenuButton
            isActive={!mobile && !!matchRoute({ to, fuzzy: true })}
            className="h-8 gap-2.5 px-2.5 text-muted-foreground select-none hover:text-foreground data-active:bg-background data-active:text-foreground data-active:shadow-[0_0_0_1px_var(--border)] data-active:hover:bg-background"
            render={<Link to={to} />}
          >
            <Icon />
            <span>{label}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      ))}
    </SidebarMenu>
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
          "flex-row items-center gap-1 px-2",
          mobile
            ? "h-[calc(3.5rem+env(safe-area-inset-top))] pt-[env(safe-area-inset-top)]"
            : "h-15 pt-2",
        )}
      >
        <SearchButton />
        <NewButton />
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
        className={cn("gap-1", mobile && "pb-[max(0.5rem,env(safe-area-inset-bottom))]")}
      >
        <ConnectionNotice />
        <SidebarNav mobile={mobile} />
        <AccountMenu />
      </SidebarFooter>
    </>
  )
}

/** Desktop: the chat list as a fixed left sidebar. */
export function ChatSidebar() {
  return (
    <Sidebar collapsible="none" className="h-svh bg-transparent">
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

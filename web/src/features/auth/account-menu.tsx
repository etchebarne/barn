import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import {
  CalendarClockIcon,
  ChevronsUpDownIcon,
  LogOutIcon,
  PlugIcon,
  SettingsIcon,
} from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { isThemePreference, useThemeStore } from "@/lib/theme"

import { authStatusQueryOptions, useLogout } from "./api"

/** Shared look for the sidebar's top (search) and bottom (account) buttons. */
export const SIDEBAR_EDGE_BUTTON =
  "flex h-9 w-full items-center gap-2 rounded-md px-2.5 text-sm outline-none select-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"

/**
 * The account button at the bottom of the sidebar: who's signed in, and a menu with Settings,
 * Connectors, theme and Log out.
 */
export function AccountMenu({ onNavigate }: { onNavigate?: () => void }) {
  const { data } = useQuery(authStatusQueryOptions)
  const navigate = useNavigate()
  const preference = useThemeStore((s) => s.preference)
  const setPreference = useThemeStore((s) => s.setPreference)
  const logout = useLogout(() => navigate({ to: "/login" }))
  const username = data?.user?.username ?? "Account"

  function go(to: "/settings" | "/connectors" | "/schedule") {
    onNavigate?.()
    void navigate({ to })
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={`${SIDEBAR_EDGE_BUTTON} hover:bg-sidebar-accent data-popup-open:bg-sidebar-accent`}
        aria-label={`Account: ${username}`}
      >
        <span
          aria-hidden="true"
          className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium uppercase"
        >
          {username.slice(0, 1)}
        </span>
        <span className="min-w-0 flex-1 truncate text-left font-medium">{username}</span>
        <ChevronsUpDownIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-(--anchor-width) min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Signed in as {username}</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => go("/settings")}>
            <SettingsIcon />
            Settings
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => go("/connectors")}>
            <PlugIcon />
            Connectors
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => go("/schedule")}>
            <CalendarClockIcon />
            Schedule
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel>Theme</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={preference}
            onValueChange={(value: unknown) => {
              if (isThemePreference(value)) setPreference(value)
            }}
          >
            <DropdownMenuRadioItem value="light">Light</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="dark">Dark</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="system">System</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled={logout.isPending} onClick={() => logout.mutate()}>
          <LogOutIcon />
          Log out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

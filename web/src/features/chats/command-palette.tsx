import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import {
  FolderPlusIcon,
  LogOutIcon,
  MonitorIcon,
  MoonIcon,
  PlugIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  SunIcon,
} from "lucide-react"
import { useEffect } from "react"

import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import { AgentAvatar, GroupAvatar, openAgentDetails, useAgentsById } from "@/features/agents"
import { SIDEBAR_EDGE_BUTTON, useLogout } from "@/features/auth"
import type { Agent, Chat } from "@/lib/api-client"
import { useThemeStore } from "@/lib/theme"

import { chatsQueryOptions } from "./api"
import { isPaletteShortcut, paletteShortcutLabel, usePaletteStore } from "./palette-store"
import { dmAgent } from "./preview"
import { useCategories } from "./sidebar-api"

/** With no query, unread chats come first, then the rest by recent activity (the list order). */
export function paletteChatOrder(chats: Chat[]): Chat[] {
  return [...chats.filter((c) => c.unreadCount > 0), ...chats.filter((c) => c.unreadCount === 0)]
}

function ChatIcon({ chat, agents }: { chat: Chat; agents: Map<string, Agent> }) {
  if (chat.kind === "dm") {
    const agent = dmAgent(chat, agents)
    return <AgentAvatar id={agent?.id} name={agent?.name ?? chat.name} size="sm" />
  }
  return <GroupAvatar memberIds={chat.members.map((m) => m.agentId)} size="sm" />
}

/**
 * The command palette (Ctrl/⌘+K anywhere, or the sidebar's search field): jump to a chat,
 * open an agent's settings, or run an action. Opens and closes without animation, since it's
 * used many times a day.
 */
export function CommandPalette() {
  const open = usePaletteStore((s) => s.open)
  const setOpen = usePaletteStore((s) => s.setOpen)
  const requestNewCategory = usePaletteStore((s) => s.requestNewCategory)
  const navigate = useNavigate()
  const { data: chats = [] } = useQuery(chatsQueryOptions)
  const { data: categories = [] } = useCategories()
  const agents = useAgentsById()
  const resolved = useThemeStore((s) => s.resolved)
  const setPreference = useThemeStore((s) => s.setPreference)
  const logout = useLogout(() => navigate({ to: "/login" }))

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (!isPaletteShortcut(event)) return
      event.preventDefault()
      usePaletteStore.getState().setOpen(!usePaletteStore.getState().open)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  const categoryName = new Map(categories.map((c) => [c.id, c.name]))

  /** Closes the palette, then runs the choice. */
  function run(action: () => void) {
    setOpen(false)
    action()
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      animated={false}
      title="Search openbot"
      description="Jump to a chat, open an agent's settings, or run an action."
      className="max-sm:top-0 max-sm:max-w-full max-sm:rounded-none! sm:max-w-lg"
    >
      <Command>
        <CommandInput placeholder="Search chats, agents and actions…" />
        <CommandList className="max-h-[min(24rem,70svh)]">
          <CommandEmpty>Nothing found.</CommandEmpty>
          {chats.length > 0 && (
            <CommandGroup heading="Chats">
              {paletteChatOrder(chats).map((chat) => {
                const category = chat.categoryId ? categoryName.get(chat.categoryId) : undefined
                return (
                  <CommandItem
                    key={chat.id}
                    value={`chat ${chat.id} ${chat.name}`}
                    keywords={[chat.name, category ?? ""]}
                    onSelect={() =>
                      run(
                        () => void navigate({ to: "/chats/$chatId", params: { chatId: chat.id } }),
                      )
                    }
                  >
                    <ChatIcon chat={chat} agents={agents} />
                    <span className="truncate">{chat.name}</span>
                    {category && (
                      <span className="truncate text-xs text-muted-foreground">{category}</span>
                    )}
                    {chat.unreadCount > 0 && (
                      <span
                        className="ml-auto rounded-full bg-primary px-1.5 text-xs text-primary-foreground tabular-nums"
                        aria-label={`${chat.unreadCount} unread`}
                      >
                        {chat.unreadCount}
                      </span>
                    )}
                  </CommandItem>
                )
              })}
            </CommandGroup>
          )}
          {agents.size > 0 && (
            <CommandGroup heading="Agents">
              {[...agents.values()].map((agent) => (
                <CommandItem
                  key={agent.id}
                  value={`agent ${agent.id} ${agent.name}`}
                  keywords={[agent.name, "settings"]}
                  onSelect={() => run(() => openAgentDetails(agent.id))}
                >
                  <AgentAvatar id={agent.id} name={agent.name} size="sm" />
                  <span className="truncate">Open {agent.name}'s settings</span>
                </CommandItem>
              ))}
              {[...agents.values()].map((agent) => (
                <CommandItem
                  key={`computer-${agent.id}`}
                  value={`computer ${agent.id} ${agent.name}`}
                  keywords={[agent.name, "computer", "files", "terminal", "sandbox"]}
                  onSelect={() =>
                    run(
                      () =>
                        void navigate({
                          to: "/agents/$agentId/computer",
                          params: { agentId: agent.id },
                        }),
                    )
                  }
                >
                  <MonitorIcon />
                  <span className="truncate">Open {agent.name}'s computer</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
          <CommandGroup heading="Actions">
            <CommandItem value="action new category" onSelect={() => run(requestNewCategory)}>
              <FolderPlusIcon />
              New category
            </CommandItem>
            <CommandItem
              value="action add connection"
              keywords={["connect", "app"]}
              onSelect={() =>
                run(() => void navigate({ to: "/connectors", search: { connector: "new" } }))
              }
            >
              <PlusIcon />
              Add connection
            </CommandItem>
            <CommandItem
              value="action connectors"
              onSelect={() => run(() => void navigate({ to: "/connectors" }))}
            >
              <PlugIcon />
              Connectors
            </CommandItem>
            <CommandItem
              value="action settings"
              onSelect={() => run(() => void navigate({ to: "/settings" }))}
            >
              <SettingsIcon />
              Settings
            </CommandItem>
            <CommandItem
              value="action toggle theme"
              keywords={["dark", "light", "appearance"]}
              onSelect={() => run(() => setPreference(resolved === "dark" ? "light" : "dark"))}
            >
              {resolved === "dark" ? <SunIcon /> : <MoonIcon />}
              Switch to {resolved === "dark" ? "light" : "dark"} theme
            </CommandItem>
            <CommandItem value="action log out" onSelect={() => run(() => logout.mutate())}>
              <LogOutIcon />
              Log out
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    </CommandDialog>
  )
}

/** The sidebar's search field: looks like an input, opens the command palette. */
export function SearchButton({ onOpen }: { onOpen?: () => void }) {
  const setOpen = usePaletteStore((s) => s.setOpen)
  return (
    <button
      type="button"
      className={`${SIDEBAR_EDGE_BUTTON} border bg-background text-muted-foreground hover:text-foreground`}
      aria-label="Search"
      aria-keyshortcuts="Control+K Meta+K"
      onClick={() => {
        onOpen?.()
        setOpen(true)
      }}
    >
      <SearchIcon className="size-4 shrink-0" aria-hidden="true" />
      <span className="flex-1 text-left">Search…</span>
      <kbd className="rounded border bg-muted px-1.5 font-sans text-[11px] text-muted-foreground [@media(hover:none)]:hidden">
        {paletteShortcutLabel()}
      </kbd>
    </button>
  )
}

import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import {
  BotIcon,
  CpuIcon,
  FolderPlusIcon,
  LogOutIcon,
  MonitorIcon,
  MoonIcon,
  PlugIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  SunIcon,
  UsersIcon,
} from "lucide-react"
import { useEffect, useState } from "react"

import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command"
import { AgentAvatar, GroupAvatar, openAgentDetails, useAgentsById } from "@/features/agents"
import { SIDEBAR_EDGE_BUTTON, useLogout } from "@/features/auth"
import type { Agent, Chat } from "@/lib/api-client"
import { useThemeStore } from "@/lib/theme"

import { chatsQueryOptions } from "./api"
import { useStartNew } from "./new-chat"
import { isPaletteShortcut, paletteShortcutLabel, usePaletteStore } from "./palette-store"
import { chatPreview, dmAgent } from "./preview"
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

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded-md border bg-muted px-1 font-sans text-[11px] text-muted-foreground">
      {children}
    </kbd>
  )
}

/**
 * The command palette (Ctrl/⌘+K anywhere, or the sidebar's search field): jump to a chat,
 * start something new, open a page, or run an action. Agents' settings and computers show up
 * once you type. Opens and closes without animation, since it's used many times a day.
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
  const { creator, start } = useStartNew()
  const [query, setQuery] = useState("")
  const searching = query.trim() !== ""

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      // Ctrl/⌘+, opens Settings, as in most desktop apps.
      if (event.key === "," && (event.metaKey || event.ctrlKey) && !event.altKey) {
        event.preventDefault()
        usePaletteStore.getState().setOpen(false)
        void navigate({ to: "/settings" })
        return
      }
      if (!isPaletteShortcut(event)) return
      event.preventDefault()
      usePaletteStore.getState().setOpen(!usePaletteStore.getState().open)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [navigate])

  const categoryName = new Map(categories.map((c) => [c.id, c.name]))
  const mod = paletteShortcutLabel().startsWith("⌘") ? "⌘" : "Ctrl"

  /** Closes the palette, then runs the choice. */
  function run(action: () => void) {
    setOpen(false)
    setQuery("")
    action()
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setQuery("")
      }}
      animated={false}
      title="Search openbot"
      description="Jump to a chat, open an agent's settings, or run an action."
      className="max-sm:top-0 max-sm:max-w-full max-sm:rounded-none sm:max-w-[40rem]"
    >
      <Command>
        <CommandInput
          placeholder="Search chats, agents and actions…"
          value={query}
          onValueChange={setQuery}
          trailing={<Kbd>esc</Kbd>}
        />
        <CommandList>
          <CommandEmpty>Nothing matches “{query}”.</CommandEmpty>
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
                    <span className="flex min-w-0 flex-1 items-baseline gap-2">
                      <span className="shrink-0 font-medium">{chat.name}</span>
                      <span className="truncate text-[13px] text-muted-foreground">
                        {chatPreview(chat, agents)}
                      </span>
                    </span>
                    {category && (
                      <span className="shrink-0 rounded-md bg-muted px-1.5 text-xs text-muted-foreground">
                        {category}
                      </span>
                    )}
                    {chat.unreadCount > 0 && (
                      <span
                        className="flex h-4.5 min-w-4.5 shrink-0 items-center justify-center rounded-full bg-brand px-1.5 text-[11px] font-semibold text-white tabular-nums"
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
          {searching && agents.size > 0 && (
            <CommandGroup heading="Agents">
              {[...agents.values()].flatMap((agent) => [
                <CommandItem
                  key={agent.id}
                  value={`agent ${agent.id} ${agent.name}`}
                  keywords={[agent.name, "settings", "agent"]}
                  onSelect={() => run(() => openAgentDetails(agent.id))}
                >
                  <AgentAvatar id={agent.id} name={agent.name} size="sm" />
                  <span className="truncate font-medium">{agent.name}</span>
                  <span className="text-[13px] text-muted-foreground">Settings</span>
                </CommandItem>,
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
                  <span className="flex size-6 items-center justify-center rounded-full bg-muted">
                    <MonitorIcon className="size-3.5" />
                  </span>
                  <span className="truncate font-medium">{agent.name}</span>
                  <span className="text-[13px] text-muted-foreground">Computer</span>
                </CommandItem>,
              ])}
            </CommandGroup>
          )}
          <CommandGroup heading="Create">
            {start && (
              <CommandItem
                value="action new agent"
                keywords={["create", "agent", "bot"]}
                onSelect={() => run(() => start("agent"))}
              >
                <BotIcon />
                New agent
                <span className="text-[13px] text-muted-foreground">
                  Ask {creator?.name ?? "an agent"}
                </span>
              </CommandItem>
            )}
            {start && (
              <CommandItem
                value="action new group"
                keywords={["create", "group", "team"]}
                onSelect={() => run(() => start("group"))}
              >
                <UsersIcon />
                New group
              </CommandItem>
            )}
            <CommandItem
              value="action add connection"
              keywords={["connect", "app", "connector"]}
              onSelect={() =>
                run(() => void navigate({ to: "/connectors", search: { connector: "new" } }))
              }
            >
              <PlusIcon />
              Add connection
            </CommandItem>
            <CommandItem value="action new category" onSelect={() => run(requestNewCategory)}>
              <FolderPlusIcon />
              New category
            </CommandItem>
          </CommandGroup>
          <CommandGroup heading="Go to">
            <CommandItem
              value="action connectors"
              keywords={["apps", "integrations"]}
              onSelect={() => run(() => void navigate({ to: "/connectors" }))}
            >
              <PlugIcon />
              Connectors
            </CommandItem>
            <CommandItem
              value="action settings"
              keywords={["preferences", "appearance", "notifications"]}
              onSelect={() => run(() => void navigate({ to: "/settings" }))}
            >
              <SettingsIcon />
              Settings
              <CommandShortcut>{mod} ,</CommandShortcut>
            </CommandItem>
            <CommandItem
              value="action models and usage"
              keywords={["provider", "api key", "opencode", "tokens", "usage", "settings"]}
              onSelect={() =>
                run(() => void navigate({ to: "/settings", search: { section: "models" } }))
              }
            >
              <CpuIcon />
              Models & usage
            </CommandItem>
          </CommandGroup>
          <CommandGroup heading="Preferences">
            <CommandItem
              value="action toggle theme"
              keywords={["dark", "light", "appearance", "theme"]}
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
        <div className="flex h-10 shrink-0 items-center gap-4 border-t px-4 text-xs text-muted-foreground select-none max-sm:hidden">
          <span className="flex items-center gap-1.5">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            to move
          </span>
          <span className="flex items-center gap-1.5">
            <Kbd>↵</Kbd>
            to open
          </span>
          <span className="ml-auto flex items-center gap-1.5">
            <Kbd>{mod}</Kbd>
            <Kbd>K</Kbd>
            anywhere
          </span>
        </div>
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
      className={`${SIDEBAR_EDGE_BUTTON} min-w-0 flex-1 border bg-background/60 text-muted-foreground shadow-[0_1px_1px_rgb(0_0_0/3%)] hover:bg-background hover:text-foreground`}
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

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
  type AnyRouter,
} from "@tanstack/react-router"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it } from "vitest"

import { SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import type { Chat } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeChat } from "@/test/fixtures"

import { CommandPalette, paletteChatOrder, SearchButton } from "./command-palette"
import { isPaletteShortcut, paletteShortcutLabel, usePaletteStore } from "./palette-store"

const chats: Chat[] = [
  makeChat({ id: "claude", name: "Claude Sessions", categoryId: "casino" }),
  makeChat({ id: "ona", name: "Ona Tester", categoryId: "casino", unreadCount: 2 }),
  makeChat({ id: "grok", name: "Grok Bot" }),
]

async function renderPalette(seed?: (client: QueryClient) => void) {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  seed?.(client)
  client.setQueryData(queryKeys.chats, chats)
  client.setQueryData(queryKeys.sidebarCategories, [
    { id: "casino", name: "casino", collapsed: false },
  ])
  client.setQueryData(queryKeys.agents, [makeAgent({ id: "agent-1", name: "Tracker" })])
  const rootRoute = createRootRoute({
    component: () => (
      <SidebarProvider>
        <TooltipProvider>
          <SearchButton />
          <CommandPalette />
        </TooltipProvider>
      </SidebarProvider>
    ),
  })
  const router: AnyRouter = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByRole("button", { name: "Search" })
  return router
}

afterEach(() => usePaletteStore.setState({ open: false, newCategoryRequest: 0, jump: null }))

describe("command palette helpers", () => {
  it("recognises Ctrl+K and ⌘+K, and labels the shortcut per platform", () => {
    const key = { key: "k", altKey: false, shiftKey: false }
    expect(isPaletteShortcut({ ...key, ctrlKey: true, metaKey: false })).toBe(true)
    expect(isPaletteShortcut({ ...key, ctrlKey: false, metaKey: true })).toBe(true)
    expect(isPaletteShortcut({ ...key, ctrlKey: false, metaKey: false })).toBe(false)
    expect(isPaletteShortcut({ ...key, ctrlKey: true, metaKey: false, shiftKey: true })).toBe(false)
    expect(paletteShortcutLabel("MacIntel")).toBe("⌘K")
    expect(paletteShortcutLabel("Linux x86_64")).toBe("Ctrl K")
  })

  it("lists unread chats first", () => {
    expect(paletteChatOrder(chats).map((c) => c.id)).toEqual(["ona", "claude", "grok"])
  })
})

describe("command palette", () => {
  it("opens with Ctrl+K and closes with Esc", async () => {
    await renderPalette()
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    await userEvent.keyboard("{Control>}k{/Control}")
    expect(await screen.findByRole("dialog")).toBeVisible()
    await userEvent.keyboard("{Escape}")
    await expect.poll(() => screen.queryByRole("dialog")).toBeNull()
  })

  it("opens from the search button", async () => {
    await renderPalette()
    await userEvent.click(screen.getByRole("button", { name: "Search" }))
    expect(await screen.findByRole("dialog")).toBeVisible()
    expect(screen.getByRole("group", { name: "Chats" })).toBeInTheDocument()
  })

  it("filters, then opens the chosen chat", async () => {
    const router = await renderPalette()
    await userEvent.click(screen.getByRole("button", { name: "Search" }))
    await userEvent.type(await screen.findByRole("combobox"), "ona")
    const options = screen.getAllByRole("option").map((o) => o.textContent)
    expect(options[0]).toContain("Ona Tester")
    expect(options.some((o) => o?.includes("Grok Bot"))).toBe(false)
    await userEvent.keyboard("{Enter}")
    expect(router.state.location.pathname).toBe("/chats/ona")
    await expect.poll(() => screen.queryByRole("dialog")).toBeNull()
  })

  it("shows category names and runs actions", async () => {
    const router = await renderPalette()
    await userEvent.click(screen.getByRole("button", { name: "Search" }))
    const chatsGroup = screen.getByRole("group", { name: "Chats" })
    expect(within(chatsGroup).getAllByText("casino")).toHaveLength(2)

    await userEvent.click(screen.getByRole("option", { name: /New category/ }))
    expect(usePaletteStore.getState().newCategoryRequest).toBe(1)

    await userEvent.click(screen.getByRole("button", { name: "Search" }))
    await userEvent.click(await screen.findByRole("option", { name: /^Settings/ }))
    expect(router.state.location.pathname).toBe("/settings")
  })
})

describe("search in the palette", () => {
  it("shows matching messages and jumps to the one picked", async () => {
    const router = await renderPalette((client) =>
      client.setQueryData(["search", "deploy"], {
        messages: [
          {
            id: "m1",
            chatId: "grok",
            author: { kind: "agent", agentId: "agent-1" },
            createdAt: new Date().toISOString(),
            snippet: "the \u0002deploy\u0003 failed",
          },
        ],
        memories: [],
        tasks: [],
      }),
    )
    await userEvent.keyboard("{Control>}k{/Control}")
    await userEvent.type(await screen.findByRole("combobox"), "deploy")
    const group = await screen.findByRole("group", { name: "Messages" })
    expect(within(group).getByText("deploy").tagName).toBe("MARK")
    expect(within(group).getByText(/Tracker in Grok Bot/)).toBeVisible()
    expect(screen.queryByText("Nothing found.")).not.toBeInTheDocument()
    // The first result is selected once it arrives, so Enter opens it.
    await userEvent.keyboard("{Enter}")
    expect(router.state.location.pathname).toBe("/chats/grok")
    expect(usePaletteStore.getState().jump).toEqual({ chatId: "grok", messageId: "m1" })
  })
})

import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Chat } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeChat } from "@/test/fixtures"

import { chatsQueryOptions } from "./api"
import { categoriesQueryOptions } from "./sidebar-api"
import type { SidebarCategory } from "./sidebar-layout"
import { NewCategory, SidebarSections } from "./sidebar-sections"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

const casino: SidebarCategory = { id: "casino", name: "casino", collapsed: false }
const chats: Chat[] = [
  makeChat({ id: "grok", name: "Grok Bot", unreadCount: 2 }),
  makeChat({
    id: "claude",
    name: "Claude Sessions",
    categoryId: "casino",
    position: 0,
    unreadCount: 1,
  }),
  makeChat({ id: "ona", name: "Ona Tester", categoryId: "casino", position: 1, unreadCount: 4 }),
]

type Call = { method: string; path: string; body: unknown }

/** Records mutating requests and answers like the server would. */
function captureRequests() {
  const calls: Call[] = []
  const respond =
    <T,>(method: string, data: T, status = 200) =>
    (...args: unknown[]) => {
      const [path, init] = args
      const body = typeof init === "object" && init && "body" in init ? init.body : undefined
      calls.push({ method, path: String(path), body })
      return Promise.resolve({ data, error: undefined, response: new Response(null, { status }) })
    }
  vi.spyOn(api, "POST").mockImplementation(
    respond("POST", { id: "new", name: "groups", collapsed: false }, 201),
  )
  vi.spyOn(api, "PATCH").mockImplementation(respond("PATCH", casino))
  vi.spyOn(api, "DELETE").mockImplementation(respond("DELETE", undefined, 204))
  vi.spyOn(api, "PUT").mockImplementation(respond("PUT", undefined, 204))
  return calls
}

function Sidebar() {
  const { data: list = [] } = useQuery(chatsQueryOptions)
  const { data: categories = [] } = useQuery(categoriesQueryOptions)
  return (
    <>
      <SidebarSections
        chats={list}
        categories={categories}
        agents={new Map()}
        isActive={() => false}
        onNavigate={() => {}}
      />
      <NewCategory />
    </>
  )
}

async function renderSidebar(categories: SidebarCategory[] = [casino]) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  client.setQueryData(queryKeys.chats, chats)
  client.setQueryData(queryKeys.sidebarCategories, categories)
  const rootRoute = createRootRoute({
    component: () => (
      <SidebarProvider>
        <TooltipProvider>
          <Sidebar />
        </TooltipProvider>
      </SidebarProvider>
    ),
  })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByText("Grok Bot")
  return client
}

afterEach(() => vi.restoreAllMocks())

describe("sidebar sections", () => {
  it("shows one plain list without categories", async () => {
    await renderSidebar([])
    expect(screen.queryByText("Unassigned")).not.toBeInTheDocument()
    expect(
      within(screen.getByRole("list", { name: "Chats" })).getAllByRole("listitem"),
    ).toHaveLength(3)
  })

  it("shows category sections with Unassigned last", async () => {
    await renderSidebar()
    const sections = screen.getAllByRole("region").map((r) => r.getAttribute("aria-label"))
    expect(sections).toEqual(["casino", "Unassigned"])
    expect(
      within(screen.getByRole("list", { name: "casino" }))
        .getAllByRole("link")
        .map((l) => l.textContent),
    ).toEqual([expect.stringContaining("Claude Sessions"), expect.stringContaining("Ona Tester")])
  })

  it("creates a category from an inline name", async () => {
    const calls = captureRequests()
    await renderSidebar()
    await userEvent.click(screen.getByRole("button", { name: "New category" }))
    await userEvent.type(
      screen.getByRole("textbox", { name: "New category name" }),
      "groups{Enter}",
    )
    expect(calls).toContainEqual({
      method: "POST",
      path: "/sidebar/categories",
      body: { name: "groups" },
    })
    expect(await screen.findByRole("region", { name: "groups" })).toBeInTheDocument()
  })

  it("renames inline: Enter saves, Escape cancels", async () => {
    const calls = captureRequests()
    await renderSidebar()
    await userEvent.click(screen.getByRole("button", { name: "casino options" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const input = await screen.findByRole("textbox", { name: "Category name" })
    await userEvent.clear(input)
    await userEvent.type(input, "work{Escape}")
    expect(calls.filter((c) => c.method === "PATCH")).toEqual([])

    await userEvent.click(screen.getByRole("button", { name: "casino options" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Rename" }))
    const again = await screen.findByRole("textbox", { name: "Category name" })
    await userEvent.clear(again)
    await userEvent.type(again, "work{Enter}")
    expect(calls).toContainEqual({
      method: "PATCH",
      path: "/sidebar/categories/{categoryId}",
      body: { name: "work" },
    })
    expect(await screen.findByRole("region", { name: "work" })).toBeInTheDocument()
  })

  it("deletes after confirming, moving its chats to Unassigned", async () => {
    const calls = captureRequests()
    const client = await renderSidebar()
    await userEvent.click(screen.getByRole("button", { name: "casino options" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete" }))
    expect(screen.getByText("Delete casino? Its chats move to Unassigned.")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Delete" }))
    expect(calls.map((c) => [c.method, c.path])).toContainEqual([
      "DELETE",
      "/sidebar/categories/{categoryId}",
    ])
    expect(client.getQueryData<Chat[]>(queryKeys.chats)?.every((c) => c.categoryId === null)).toBe(
      true,
    )
  })

  it("collapses a category and shows its unread total on the header", async () => {
    const calls = captureRequests()
    await renderSidebar()
    await userEvent.click(screen.getByRole("button", { name: "casino", expanded: true }))
    expect(calls).toContainEqual({
      method: "PATCH",
      path: "/sidebar/categories/{categoryId}",
      body: { collapsed: true },
    })
    expect(screen.queryByText("Claude Sessions")).not.toBeInTheDocument()
    expect(screen.getByLabelText("5 unread in casino")).toHaveTextContent("5")
    expect(screen.getByRole("button", { name: "casino" })).toHaveAttribute("aria-expanded", "false")
  })

  it("toggles from anywhere on the header, but not from its ⋯ menu", async () => {
    const calls = captureRequests()
    await renderSidebar()
    // The name itself (not only the chevron).
    await userEvent.click(
      within(screen.getByRole("button", { name: "casino" })).getByText("casino"),
    )
    expect(calls.filter((c) => c.method === "PATCH")).toEqual([
      { method: "PATCH", path: "/sidebar/categories/{categoryId}", body: { collapsed: true } },
    ])

    await userEvent.click(screen.getByRole("button", { name: "casino options" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Rename" }))
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(1)
    // Typing a new name doesn't toggle either.
    await userEvent.click(screen.getByRole("textbox", { name: "Category name" }))
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(1)
  })

  it("toggles Unassigned with Enter on its header, kept on this device", async () => {
    await renderSidebar()
    const header = screen.getByRole("button", { name: "Unassigned", expanded: true })
    header.focus()
    await userEvent.keyboard("{Enter}")
    expect(screen.getByRole("button", { name: "Unassigned" })).toHaveAttribute(
      "aria-expanded",
      "false",
    )
    expect(screen.queryByText("Grok Bot")).not.toBeInTheDocument()
    await userEvent.keyboard(" ")
    expect(await screen.findByText("Grok Bot")).toBeInTheDocument()
  })

  it("moves a category with Move down in its menu", async () => {
    const calls = captureRequests()
    await renderSidebar([casino, { id: "work", name: "work", collapsed: false }])
    await userEvent.click(screen.getByRole("button", { name: "casino options" }))
    expect(screen.queryByRole("menuitem", { name: "Move up" })).not.toBeInTheDocument()
    await userEvent.click(await screen.findByRole("menuitem", { name: "Move down" }))
    expect(calls).toContainEqual(
      expect.objectContaining({
        method: "PUT",
        body: expect.objectContaining({ categoryOrder: ["work", "casino"] }),
      }),
    )
  })

  it("moves a chat with Move to and saves both sections", async () => {
    const calls = captureRequests()
    const client = await renderSidebar()
    await userEvent.click(screen.getByRole("button", { name: "More for Grok Bot" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Move to" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "casino" }))
    await waitFor(() =>
      expect(calls).toContainEqual({
        method: "PUT",
        path: "/sidebar/layout",
        body: {
          categoryOrder: ["casino"],
          sections: [
            { categoryId: null, chatIds: [] },
            { categoryId: "casino", chatIds: ["claude", "ona", "grok"] },
          ],
        },
      }),
    )
    expect(
      client.getQueryData<Chat[]>(queryKeys.chats)?.find((c) => c.id === "grok"),
    ).toMatchObject({
      categoryId: "casino",
      position: 2,
    })
  })
})

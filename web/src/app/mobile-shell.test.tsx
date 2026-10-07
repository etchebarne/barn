import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
  type AnyRouter,
} from "@tanstack/react-router"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { PageHeader } from "@/components/page-header"
import { SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { firstChatRedirect } from "@/lib/mobile-nav"
import { queryKeys } from "@/lib/query-keys"
import { makeChat } from "@/test/fixtures"

import { MobileShell } from "./app-shell"

const chats = [
  makeChat({ id: "c1", name: "Claude Sessions", unreadCount: 3 }),
  makeChat({ id: "c2", name: "Grok Bot" }),
]

function ChatScreen() {
  return (
    <PageHeader>
      <h1>Chat screen</h1>
    </PageHeader>
  )
}

async function renderMobile(initial = "/") {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(queryKeys.chats, chats)
  client.setQueryData(queryKeys.sidebarCategories, [])
  client.setQueryData(queryKeys.agents, [])
  client.setQueryData(queryKeys.authStatus, {
    setupRequired: false,
    user: { id: "u", username: "martin" },
  })
  const rootRoute = createRootRoute({ component: Outlet })
  const shell = createRoute({
    getParentRoute: () => rootRoute,
    id: "shell",
    component: () => (
      <SidebarProvider>
        <TooltipProvider>
          <MobileShell />
        </TooltipProvider>
      </SidebarProvider>
    ),
  })
  const index = createRoute({ getParentRoute: () => shell, path: "/", component: () => null })
  const chat = createRoute({
    getParentRoute: () => shell,
    path: "/chats/$chatId",
    component: ChatScreen,
  })
  const router: AnyRouter = createRouter({
    routeTree: rootRoute.addChildren([shell.addChildren([index, chat])]),
    history: createMemoryHistory({ initialEntries: [initial] }),
  })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByText("Grok Bot")
  return router
}

beforeEach(() => {
  // A phone-sized viewport.
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: query.includes("max-width: 767px"),
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  )
})
afterEach(() => vi.restoreAllMocks())

describe("mobile navigation", () => {
  it("shows the chat list at / instead of opening the first chat", async () => {
    expect(firstChatRedirect(true, chats)).toBeNull()
    expect(firstChatRedirect(false, chats)).toBe("c1")

    const router = await renderMobile("/")
    expect(router.state.location.pathname).toBe("/")
    const row = screen.getByRole("link", { name: /Claude Sessions/ })
    expect(row).toBeVisible()
    expect(within(row).getByText("3")).toBeVisible()
    expect(screen.queryByText("Chat screen")).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Toggle sidebar" })).not.toBeInTheDocument()
  })

  it("pushes a chat over the list, and Back returns to the list", async () => {
    const router = await renderMobile("/")
    await userEvent.click(screen.getByRole("link", { name: /Grok Bot/ }))
    expect(router.state.location.pathname).toBe("/chats/c2")
    expect(await screen.findByText("Chat screen")).toBeVisible()

    await userEvent.click(screen.getByRole("button", { name: "Back" }))
    await waitFor(() => expect(router.state.location.pathname).toBe("/"))
    expect(screen.queryByText("Chat screen")).not.toBeInTheDocument()
  })

  it("goes to the list on Back from a deep link with no history", async () => {
    const router = await renderMobile("/chats/c1")
    expect(await screen.findByText("Chat screen")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Back" }))
    await waitFor(() => expect(router.state.location.pathname).toBe("/"))
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router"
import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { SettingsPage } from "@/features/settings"
import { api, type Agent, type Chat } from "@/lib/api-client"
import { setChatView } from "@/lib/chat-view"
import type { DesktopBridge } from "@/lib/desktop"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeChat, makeMessage } from "@/test/fixtures"

import { desktopNotificationFor, notifyDesktop, useDesktopIntegration } from "./desktop"

function mockBridge() {
  let navigate: ((path: string) => void) | null = null
  const bridge = {
    platform: "linux",
    notify: vi.fn<DesktopBridge["notify"]>(),
    setUnread: vi.fn<DesktopBridge["setUnread"]>(),
    changeServer: vi.fn<DesktopBridge["changeServer"]>(),
    onNavigate: vi.fn<DesktopBridge["onNavigate"]>((callback) => {
      navigate = callback
      return () => {}
    }),
  }
  window.openbotDesktop = bridge
  return { bridge, navigate: (path: string) => navigate?.(path) }
}

const scout = makeAgent({ id: "scout", name: "Scout", notifications: true })
const quiet = makeAgent({ id: "quiet", name: "Quiet", notifications: false })
const dm = makeChat({ id: "dm-scout", name: "Scout", unreadCount: 2 })
const group = makeChat({ id: "team", kind: "group", name: "Launch crew", unreadCount: 3 })

function seededClient() {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData<Agent[]>(queryKeys.agents, [scout, quiet])
  client.setQueryData<Chat[]>(queryKeys.chats, [dm, group])
  return client
}

beforeEach(() => vi.spyOn(document, "hasFocus").mockReturnValue(true))
afterEach(() => {
  delete window.openbotDesktop
  setChatView(null)
  vi.restoreAllMocks()
})

describe("desktop notifications", () => {
  const context = { agents: [scout, quiet], chats: [dm, group], focused: true, openChat: null }

  it("notify for an agent's message in a chat that isn't open", () => {
    const message = makeMessage({
      chatId: "dm-scout",
      author: { kind: "agent", agentId: "scout" },
      body: "**Done**: see the [report](https://x.test)",
    })
    expect(desktopNotificationFor(message, context)).toEqual({
      title: "Scout",
      body: "Done: see the report",
      chatId: "dm-scout",
      tag: message.id,
    })
  })

  it("name the group, and describe files and questions", () => {
    const file = makeMessage({
      chatId: "team",
      author: { kind: "agent", agentId: "scout" },
      body: "",
      attachments: [
        { id: "a", name: "x.png", mime: "image/png", size: 1, url: "/x", width: 1, height: 1 },
      ],
    })
    expect(desktopNotificationFor(file, context)).toMatchObject({
      title: "Scout in Launch crew",
      body: "Sent a file",
    })
    const prompt = makeMessage({
      author: { kind: "agent", agentId: "scout" },
      prompt: {
        kind: "single",
        question: "Pick one",
        options: [],
        allowOther: false,
        status: "pending",
        answer: null,
      },
    })
    expect(desktopNotificationFor(prompt, context)?.body).toBe("Asked you a question")
  })

  it("stay quiet for the open, focused chat; the user's own messages; and muted agents", () => {
    const fromScout = makeMessage({
      chatId: "dm-scout",
      author: { kind: "agent", agentId: "scout" },
    })
    expect(desktopNotificationFor(fromScout, { ...context, openChat: "dm-scout" })).toBeNull()
    // Open but the window isn't focused: notify.
    expect(
      desktopNotificationFor(fromScout, { ...context, openChat: "dm-scout", focused: false }),
    ).not.toBeNull()
    expect(
      desktopNotificationFor(makeMessage({ author: { kind: "user", agentId: null } }), context),
    ).toBeNull()
    expect(
      desktopNotificationFor(makeMessage({ author: { kind: "agent", agentId: "quiet" } }), context),
    ).toBeNull()
  })

  it("go through the bridge, unless turned off on this device", () => {
    const { bridge } = mockBridge()
    const client = seededClient()
    vi.spyOn(document, "hasFocus").mockReturnValue(false)
    setChatView({ chatId: "dm-scout", atLatest: true })
    const message = makeMessage({ chatId: "dm-scout", author: { kind: "agent", agentId: "scout" } })
    notifyDesktop(client, message)
    expect(bridge.notify).toHaveBeenCalledTimes(1)

    window.localStorage.setItem("openbot:desktop:notifications", "0")
    notifyDesktop(client, message)
    expect(bridge.notify).toHaveBeenCalledTimes(1)
  })
})

function Integration() {
  useDesktopIntegration()
  return <Outlet />
}

async function renderWithRouter(client: QueryClient, page: () => React.ReactNode = () => null) {
  const rootRoute = createRootRoute({ component: Integration })
  const routes = [
    createRoute({ getParentRoute: () => rootRoute, path: "/", component: page }),
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/chats/$chatId",
      component: () => <p>chat page</p>,
    }),
  ]
  const router = createRouter({
    routeTree: rootRoute.addChildren(routes),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await waitFor(() => expect(router.state.status).toBe("idle"))
  return router
}

describe("desktop integration", () => {
  it("keeps the unread badge in sync with the chat list", async () => {
    const { bridge } = mockBridge()
    const client = seededClient()
    await renderWithRouter(client)
    await waitFor(() => expect(bridge.setUnread).toHaveBeenLastCalledWith(5))
    act(() => {
      client.setQueryData<Chat[]>(queryKeys.chats, [{ ...dm, unreadCount: 0 }, group])
    })
    await waitFor(() => expect(bridge.setUnread).toHaveBeenLastCalledWith(3))
  })

  it("routes to the paths the app asks for (a clicked notification)", async () => {
    const desktopApp = mockBridge()
    const router = await renderWithRouter(seededClient())
    act(() => {
      desktopApp.navigate("/chats/dm-scout")
    })
    expect(await screen.findByText("chat page")).toBeVisible()
    expect(router.state.location.pathname).toBe("/chats/dm-scout")
  })

  it("does nothing in a browser", async () => {
    await renderWithRouter(seededClient())
    expect(window.openbotDesktop).toBeUndefined()
  })
})

function renderSettings() {
  // Sections that load from the server stay loading; this is about which sections show.
  vi.spyOn(api, "GET").mockImplementation(() => new Promise(() => {}))
  const client = seededClient()
  return renderWithRouter(client, () => <SettingsPage onLoggedOut={() => Promise.resolve()} />)
}
describe("settings", () => {
  it("in the desktop app: the notifications toggle and the server it's connected to", async () => {
    const { bridge } = mockBridge()
    await renderSettings()
    const toggle = await screen.findByRole("switch", { name: "Desktop notifications" })
    expect(toggle).toBeChecked()
    await userEvent.click(toggle)
    expect(window.localStorage.getItem("openbot:desktop:notifications")).toBe("0")
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument()
    expect(screen.getByText(window.location.origin)).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Change server…" }))
    expect(bridge.changeServer).toHaveBeenCalled()
  })

  it("in a browser: Web Push settings and a link to the desktop app", async () => {
    await renderSettings()
    expect(await screen.findByRole("link", { name: /Get the desktop app/ })).toHaveAttribute(
      "href",
      "https://github.com/etchebarne/openbot/releases/latest",
    )
    expect(screen.queryByRole("switch", { name: "Desktop notifications" })).not.toBeInTheDocument()
  })
})

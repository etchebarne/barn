import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { toast } from "sonner"
import { afterEach, describe, expect, it, vi } from "vitest"

import { SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Chat } from "@/lib/api-client"
import { flattenMessages, type MessagesData } from "@/lib/chat-cache"
import { queryKeys } from "@/lib/query-keys"
import { applyWsEvent } from "@/lib/ws"
import { makeChat, makeMessage } from "@/test/fixtures"

import { pendingStore } from "./pending-store"
import { useReplyStore } from "./reply-store"
import type { SidebarCategory } from "./sidebar-layout"
import { SidebarSections } from "./sidebar-sections"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

const dmMessage = makeMessage({ id: "01DM", chatId: "dm", body: "hello" })
const groupMessage = makeMessage({ id: "01GROUP", chatId: "group", body: "team" })
const chats: Chat[] = [
  makeChat({ id: "dm", name: "Weekly Digest", unreadCount: 3, lastMessage: dmMessage }),
  makeChat({
    id: "group",
    kind: "group",
    name: "Launch crew",
    members: [
      { agentId: "agent-1", position: 0 },
      { agentId: "agent-2", position: 1 },
    ],
    lastMessage: groupMessage,
  }),
]
const casino: SidebarCategory = { id: "casino", name: "casino", collapsed: false }

function page(message: (typeof chats)[number]["lastMessage"]): MessagesData {
  return {
    pages: [{ messages: message ? [message] : [], hasMore: false }],
    pageParams: [undefined],
  }
}

function seed() {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  client.setQueryData(queryKeys.chats, chats)
  client.setQueryData(queryKeys.messages("dm"), page(dmMessage))
  client.setQueryData(queryKeys.messages("group"), page(groupMessage))
  return client
}

async function renderSidebar(categories: SidebarCategory[]) {
  const client = seed()
  const rootRoute = createRootRoute({
    component: () => (
      <SidebarProvider>
        <TooltipProvider>
          <SidebarSections
            chats={chats}
            categories={categories}
            agents={new Map()}
            isActive={() => false}
            onNavigate={() => {}}
          />
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
  await screen.findByText("Weekly Digest")
  return client
}

const messagesOf = (client: QueryClient, chatId: string) =>
  flattenMessages(client.getQueryData<MessagesData>(queryKeys.messages(chatId))).map((m) => m.id)

afterEach(() => vi.restoreAllMocks())

describe("clear history", () => {
  it("is offered on DMs only", async () => {
    await renderSidebar([casino])
    await userEvent.click(screen.getByRole("button", { name: "More for Launch crew" }))
    expect(await screen.findByRole("menuitem", { name: /Move to/ })).toBeInTheDocument()
    expect(screen.queryByRole("menuitem", { name: "Clear history" })).not.toBeInTheDocument()
    await userEvent.keyboard("{Escape}")

    await userEvent.click(screen.getByRole("button", { name: "More for Weekly Digest" }))
    expect(await screen.findByRole("menuitem", { name: "Clear history" })).toBeInTheDocument()
  })

  it("keeps the menu on DMs when there are no categories to move to", async () => {
    await renderSidebar([])
    expect(screen.queryByRole("button", { name: "More for Launch crew" })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole("button", { name: "More for Weekly Digest" }))
    expect(await screen.findByRole("menuitem", { name: "Clear history" })).toBeInTheDocument()
    expect(screen.queryByRole("menuitem", { name: /Move to/ })).not.toBeInTheDocument()
  })

  it("clears after confirming: calls the endpoint, empties the DM, toasts", async () => {
    const calls: string[] = []
    vi.spyOn(api, "DELETE").mockImplementation((...args: unknown[]) => {
      const [path, init] = args
      calls.push(`${String(path)} ${JSON.stringify(init)}`)
      return Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(null, { status: 204 }),
      })
    })
    const client = await renderSidebar([])
    useReplyStore.getState().startReply(dmMessage)

    await userEvent.click(screen.getByRole("button", { name: "More for Weekly Digest" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Clear history" }))
    const dialog = await screen.findByRole("alertdialog")
    expect(dialog).toHaveTextContent("Clear your DM with Weekly Digest?")
    expect(dialog).toHaveTextContent("Weekly Digest forgets the conversation")
    await userEvent.click(screen.getByRole("button", { name: "Clear history" }))

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Cleared your DM with Weekly Digest"),
    )
    expect(calls).toEqual([expect.stringMatching(/^\/chats\/\{chatId\}\/history .*"chatId":"dm"/)])
    expect(messagesOf(client, "dm")).toEqual([])
    expect(messagesOf(client, "group")).toEqual(["01GROUP"])
    const dm = client.getQueryData<Chat[]>(queryKeys.chats)?.find((c) => c.id === "dm")
    expect(dm).toMatchObject({ unreadCount: 0, lastMessage: null })
    expect(useReplyStore.getState().byChat.dm).toBeNull()
  })

  it("shows an error toast and keeps the messages when clearing fails", async () => {
    vi.spyOn(api, "DELETE").mockImplementation(() =>
      Promise.resolve({
        data: undefined,
        error: { message: "Couldn't clear it" },
        response: new Response(null, { status: 500 }),
      }),
    )
    const client = await renderSidebar([])
    await userEvent.click(screen.getByRole("button", { name: "More for Weekly Digest" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Clear history" }))
    await userEvent.click(await screen.findByRole("button", { name: "Clear history" }))
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
    expect(messagesOf(client, "dm")).toEqual(["01DM"])
  })
})

describe("chat.cleared", () => {
  it("empties that chat's messages, preview, unread, reply and pending sends only", () => {
    const client = seed()
    useReplyStore.getState().startReply(dmMessage)
    useReplyStore.getState().startReply(groupMessage)
    pendingStore
      .getState()
      .add("dm", { clientId: "c1", body: "hi", status: "failed", createdAt: 1 })
    pendingStore
      .getState()
      .add("group", { clientId: "c2", body: "yo", status: "sending", createdAt: 1 })

    applyWsEvent(client, { type: "chat.cleared", chatId: "dm" })

    expect(messagesOf(client, "dm")).toEqual([])
    expect(messagesOf(client, "group")).toEqual(["01GROUP"])
    const list = client.getQueryData<Chat[]>(queryKeys.chats) ?? []
    expect(list.find((c) => c.id === "dm")).toMatchObject({ unreadCount: 0, lastMessage: null })
    expect(list.find((c) => c.id === "group")?.lastMessage?.id).toBe("01GROUP")
    expect(useReplyStore.getState().byChat.dm).toBeNull()
    expect(useReplyStore.getState().byChat.group?.id).toBe("01GROUP")
    expect(pendingStore.getState().byChat.dm).toBeUndefined()
    expect(pendingStore.getState().byChat.group).toHaveLength(1)
  })
})

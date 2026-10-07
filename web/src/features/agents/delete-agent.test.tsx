import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api, type Agent, type Chat } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeChat } from "@/test/fixtures"

import { useAgentDetailsStore } from "./details-store"
import { DangerZone } from "./settings-sections"

const toast = vi.hoisted(() => ({
  success: vi.fn<(text: string) => void>(),
  error: vi.fn<(text: string) => void>(),
}))
vi.mock("sonner", () => ({ toast }))

const tracker = makeAgent({ id: "tracker", name: "Tracker", isAdmin: false })
const dm = makeChat({
  id: "dm-tracker",
  name: "Tracker",
  members: [{ agentId: "tracker", position: 0 }],
})

function deleteResponds(status: number, message?: string) {
  const response = new Response(null, { status })
  return vi
    .spyOn(api, "DELETE")
    .mockImplementation(() =>
      Promise.resolve(
        message
          ? { data: undefined, error: { message }, response }
          : { data: undefined, error: undefined, response },
      ),
    )
}

async function renderDangerZone(path = "/chats/dm-tracker") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData<Agent[]>(queryKeys.agents, [makeAgent(), tracker])
  client.setQueryData<Chat[]>(queryKeys.chats, [makeChat(), dm])
  const rootRoute = createRootRoute({ component: Outlet })
  const page = () => <DangerZone agent={tracker} onArchived={vi.fn<() => void>()} />
  const routes = [
    createRoute({ getParentRoute: () => rootRoute, path: "/", component: () => <p>home</p> }),
    createRoute({ getParentRoute: () => rootRoute, path: "/chats/$chatId", component: page }),
  ]
  const router = createRouter({
    routeTree: rootRoute.addChildren(routes),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByText("Danger zone")
  return { client, router }
}

afterEach(() => {
  vi.restoreAllMocks()
  useAgentDetailsStore.getState().close()
})

describe("Delete agent", () => {
  it("confirms inline, deletes, and leaves the agent's DM", async () => {
    const del = deleteResponds(204)
    useAgentDetailsStore.getState().open("tracker")
    const { client, router } = await renderDangerZone()

    expect(
      screen.getByText(/Removes Tracker, its DM, memories, tasks and files for good/),
    ).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Delete agent" }))
    expect(del).not.toHaveBeenCalled()
    expect(
      screen.getByRole("alertdialog", { name: "Delete Tracker? This can't be undone." }),
    ).toBeVisible()

    await userEvent.click(screen.getByRole("button", { name: "Delete" }))

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Deleted Tracker"))
    expect(del).toHaveBeenCalledWith("/agents/{agentId}", {
      params: { path: { agentId: "tracker" } },
    })
    expect(client.getQueryData<Agent[]>(queryKeys.agents)?.map((a) => a.id)).toEqual(["agent-1"])
    expect(client.getQueryData<Chat[]>(queryKeys.chats)?.map((c) => c.id)).toEqual(["chat-1"])
    expect(useAgentDetailsStore.getState().agentId).toBeNull()
    await waitFor(() => expect(router.state.location.pathname).toBe("/"))
  })

  it("cancels without deleting", async () => {
    const del = deleteResponds(204)
    await renderDangerZone()
    await userEvent.click(screen.getByRole("button", { name: "Delete agent" }))
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }))
    expect(del).not.toHaveBeenCalled()
    expect(screen.getByRole("button", { name: "Delete agent" })).toBeVisible()
  })

  it("shows the last-admin 409 inline and keeps the agent", async () => {
    deleteResponds(409, "openbot needs at least one admin agent, so this one can't be deleted")
    const { client } = await renderDangerZone()
    await userEvent.click(screen.getByRole("button", { name: "Delete agent" }))
    await userEvent.click(screen.getByRole("button", { name: "Delete" }))

    expect(
      await screen.findByText(
        "openbot needs at least one admin agent, so this one can't be deleted",
      ),
    ).toBeVisible()
    expect(client.getQueryData<Agent[]>(queryKeys.agents)).toHaveLength(2)
    expect(toast.success).not.toHaveBeenCalled()
  })
})

describe("Clear history", () => {
  it("confirms inline, then clears the agent's DM", async () => {
    const del = deleteResponds(204)
    const { client } = await renderDangerZone()
    await userEvent.click(await screen.findByRole("button", { name: "Clear history" }))
    expect(screen.getByRole("alertdialog", { name: "Clear your DM with Tracker?" })).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Clear history" }))

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Cleared your DM with Tracker"))
    expect(del).toHaveBeenCalledWith("/chats/{chatId}/history", {
      params: { path: { chatId: "dm-tracker" } },
    })
    expect(
      client.getQueryData<Chat[]>(queryKeys.chats)?.find((c) => c.id === "dm-tracker"),
    ).toMatchObject({ unreadCount: 0, lastMessage: null })
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument()
  })
})

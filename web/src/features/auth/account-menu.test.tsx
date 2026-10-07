import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
  type AnyRouter,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { useThemeStore } from "@/lib/theme"

import { AccountMenu } from "./account-menu"

async function renderMenu() {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(queryKeys.authStatus, {
    setupRequired: false,
    user: { id: "u1", username: "martin" },
  })
  const rootRoute = createRootRoute({ component: () => <AccountMenu /> })
  const router: AnyRouter = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByRole("button", { name: "Account: martin" })
  return router
}

afterEach(() => vi.restoreAllMocks())

describe("AccountMenu", () => {
  it("shows the user and the menu items", async () => {
    await renderMenu()
    expect(screen.getByText("martin")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Account: martin" }))
    const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent)
    expect(items).toEqual(["Settings", "Connectors", "Log out"])
    expect(screen.getAllByRole("menuitemradio").map((i) => i.textContent)).toEqual([
      "Light",
      "Dark",
      "System",
    ])
  })

  it("navigates to Settings and Connectors", async () => {
    const router = await renderMenu()
    await userEvent.click(screen.getByRole("button", { name: "Account: martin" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Connectors" }))
    expect(router.state.location.pathname).toBe("/connectors")
    await userEvent.click(screen.getByRole("button", { name: "Account: martin" }))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Settings" }))
    expect(router.state.location.pathname).toBe("/settings")
  })

  it("switches the theme and logs out", async () => {
    const post = vi.spyOn(api, "POST").mockImplementation(() =>
      Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(null, { status: 204 }),
      }),
    )
    const router = await renderMenu()
    await userEvent.click(screen.getByRole("button", { name: "Account: martin" }))
    await userEvent.click(await screen.findByRole("menuitemradio", { name: "Dark" }))
    expect(useThemeStore.getState().preference).toBe("dark")

    // Picking a theme keeps the menu open.
    await userEvent.click(await screen.findByRole("menuitem", { name: "Log out" }))
    expect(post).toHaveBeenCalledWith("/auth/logout")
    await expect.poll(() => router.state.location.pathname).toBe("/login")
  })
})

import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { applyWsEvent } from "@/lib/ws"

import { categoriesQueryOptions, useUpdateCategory } from "./sidebar-api"
import type { SidebarCategory } from "./sidebar-layout"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

afterEach(() => vi.restoreAllMocks())

describe("sidebar writes", () => {
  it("rapid collapse toggles end in the last local state without flipping back", async () => {
    // The server applies each PATCH when it's answered; until then it still has the old value.
    let server: SidebarCategory = { id: "casino", name: "casino", collapsed: false }
    const answers: (() => void)[] = []
    vi.spyOn(api, "PATCH").mockImplementation((...args: unknown[]) => {
      const init = args[1]
      const body = typeof init === "object" && init && "body" in init ? init.body : {}
      return new Promise((resolve) => {
        answers.push(() => {
          server = Object.assign({}, server, body)
          resolve({ data: server, error: undefined, response: new Response(null, { status: 200 }) })
        })
      })
    })
    const gets = vi.spyOn(api, "GET").mockImplementation(() =>
      Promise.resolve({
        data: [server],
        error: undefined,
        response: new Response(null, { status: 200 }),
      }),
    )

    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    client.setQueryData(queryKeys.sidebarCategories, [server])
    client.setQueryData(queryKeys.chats, [])
    // Keep the categories query observed, like the sidebar does, so invalidations refetch.
    const observer = new QueryObserver(client, { ...categoriesQueryOptions, staleTime: Infinity })
    const unsubscribe = observer.subscribe(() => {})
    const queryHash = client
      .getQueryCache()
      .find({ queryKey: queryKeys.sidebarCategories })?.queryHash
    const seen: boolean[] = []
    const stop = client.getQueryCache().subscribe((event) => {
      if (event.query.queryHash !== queryHash) return
      const value = client.getQueryData<SidebarCategory[]>(queryKeys.sidebarCategories)?.[0]
        ?.collapsed
      if (value !== undefined && seen.at(-1) !== value) seen.push(value)
    })

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useUpdateCategory(), { wrapper })

    // Three quick toggles: collapse, expand, collapse.
    await act(async () => {
      result.current.mutate({ id: "casino", collapsed: true })
      await Promise.resolve()
    })
    await act(async () => {
      result.current.mutate({ id: "casino", collapsed: false })
      await Promise.resolve()
    })
    await act(async () => {
      result.current.mutate({ id: "casino", collapsed: true })
      await Promise.resolve()
    })
    // The server announces the first change while the others are still pending.
    await act(async () => {
      answers[0]?.()
      applyWsEvent(client, { type: "sidebar.updated" })
      await Promise.resolve()
    })
    await act(async () => {
      answers[1]?.()
      applyWsEvent(client, { type: "sidebar.updated" })
      answers[2]?.()
      await Promise.resolve()
    })

    await waitFor(() => expect(gets).toHaveBeenCalled())
    await waitFor(() =>
      expect(
        client.getQueryData<SidebarCategory[]>(queryKeys.sidebarCategories)?.[0]?.collapsed,
      ).toBe(true),
    )
    // Only the user's own flips, never a server state from in between.
    expect(seen).toEqual([true, false, true])
    // One refetch, after the last write.
    expect(gets).toHaveBeenCalledTimes(1)
    stop()
    unsubscribe()
  })
})

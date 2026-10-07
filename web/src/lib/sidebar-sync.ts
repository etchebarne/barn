import type { QueryClient } from "@tanstack/react-query"

import { queryKeys } from "./query-keys"

/**
 * Keeps the sidebar from "reconciling" visibly while the user changes it quickly.
 *
 * Every sidebar write (collapse, rename, reorder, …) applies optimistically. While any write is
 * in flight, refetches of the sidebar's data would bring back the server's state from *before*
 * the newer local changes, so: writes cancel refetches already running, `sidebar.updated`
 * events are ignored while writes are pending, and one refetch runs after the last write
 * settles, by which point the server has everything.
 */
let inFlight = 0

export function sidebarBusy(): boolean {
  return inFlight > 0
}

/** Call at the start of a sidebar write, before applying the optimistic change. */
export async function sidebarWriteStarted(queryClient: QueryClient): Promise<void> {
  inFlight += 1
  await Promise.all([
    queryClient.cancelQueries({ queryKey: queryKeys.sidebarCategories }),
    queryClient.cancelQueries({ queryKey: queryKeys.chats, exact: true }),
  ])
}

/** Call when a sidebar write settles (success or error). */
export function sidebarWriteSettled(queryClient: QueryClient): void {
  inFlight = Math.max(0, inFlight - 1)
  if (inFlight > 0) return
  void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarCategories })
  void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true })
}

/** Refetch the sidebar after someone changed it, unless our own writes are still pending. */
export function sidebarChangedRemotely(queryClient: QueryClient): void {
  // Our pending writes will refetch when they settle; refetching now would flash old state.
  if (sidebarBusy()) return
  void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarCategories })
  void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true })
}

import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type AuthStatus, type User } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export const authStatusQueryOptions = queryOptions({
  queryKey: queryKeys.authStatus,
  queryFn: () => unwrap(api.GET("/auth/status")),
  staleTime: Number.POSITIVE_INFINITY,
})

type Credentials = { username: string; password: string }

/** Account setup and login both start a session; afterwards everything else is refetched. */
function useStartSession(start: (credentials: Credentials) => Promise<User>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: start,
    onSuccess: (user) => {
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== "auth" })
      queryClient.setQueryData<AuthStatus>(queryKeys.authStatus, { setupRequired: false, user })
    },
  })
}

export function useSetupAccount() {
  return useStartSession((body) => unwrap(api.POST("/auth/setup", { body })))
}

export function useLogin() {
  return useStartSession((body) => unwrap(api.POST("/auth/login", { body })))
}

/**
 * Logs out, then calls `leave` (navigate to the login screen) before dropping cached data, so
 * nothing still mounted refetches with a dead session.
 */
export function useLogout(leave: () => Promise<void>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST("/auth/logout")),
    onSettled: async () => {
      queryClient.setQueryData<AuthStatus>(queryKeys.authStatus, {
        setupRequired: false,
        user: null,
      })
      await leave()
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== "auth" })
    },
  })
}

import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Schemas } from "./api-client"
import { queryKeys } from "./query-keys"

/** An agent's computer: its Docker sandbox. Shared by the agent sheet and the Computer page. */
export type Sandbox = Schemas["Sandbox"]
export type SandboxStatus = Sandbox["status"]

/** Where the agent's own files live (and where the terminal starts). */
export const SANDBOX_HOME = "/home/agent"

export function sandboxQueryOptions(agentId: string) {
  return queryOptions({
    queryKey: queryKeys.sandbox(agentId),
    queryFn: () => unwrap(api.GET("/agents/{agentId}/sandbox", { params: { path: { agentId } } })),
    // Its state changes outside the app (the agent's tools start it); refresh on every visit.
    staleTime: 0,
  })
}

export function useSandbox(agentId: string) {
  return useQuery(sandboxQueryOptions(agentId))
}

/** Restarts the computer: running programs stop, files stay. */
export function useRestartSandbox(agentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      unwrap(api.POST("/agents/{agentId}/sandbox/restart", { params: { path: { agentId } } })),
    onSuccess: (sandbox) => queryClient.setQueryData(queryKeys.sandbox(agentId), sandbox),
  })
}

/** "Running", "Stopped", … for the status dot's label. */
export function sandboxStatusLabel(status: SandboxStatus | "restarting"): string {
  switch (status) {
    case "running":
      return "Running"
    case "stopped":
      return "Stopped"
    case "none":
      return "Not started yet"
    case "restarting":
      return "Restarting…"
    default:
      return "Unavailable"
  }
}

/** The confirm copy for Restart, shared by the sheet and the Computer page. */
export const RESTART_COPY =
  "Programs running on it stop, including open terminals. Files stay where they are."

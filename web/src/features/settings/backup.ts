import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export type AgentsBackup = Schemas["AgentsBackup"]
export type RestoreResult = Schemas["RestoreResult"]

/** Reads a backup file, or says why it isn't one. */
export function parseBackup(text: string): AgentsBackup | string {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    return "That file isn't JSON."
  }
  if (
    typeof data !== "object" ||
    data === null ||
    !("format" in data) ||
    data.format !== "openbot-agents" ||
    !("agents" in data) ||
    !Array.isArray(data.agents)
  ) {
    return "That file isn't an openbot agents backup."
  }
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- format checked above; the server validates the rest
  return data as AgentsBackup
}

/** Which agents in a backup would be restored, and which skipped because the name is taken. */
export function planRestore(
  backup: AgentsBackup,
  existingNames: string[],
): { name: string; restore: boolean }[] {
  const taken = new Set(existingNames.map((n) => n.trim().toLowerCase()))
  return backup.agents.map((agent) => {
    const key = agent.name.trim().toLowerCase()
    const restore = !taken.has(key)
    taken.add(key)
    return { name: agent.name, restore }
  })
}

/** Downloads every agent's configuration as a JSON file. */
export function useExportAgents() {
  return useMutation({
    mutationFn: async () => {
      const backup = await unwrap(api.GET("/backup/agents"))
      const blob = new Blob([JSON.stringify(backup, null, 2)], { type: "application/json" })
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = `openbot-agents-${backup.exportedAt.slice(0, 10)}.json`
      link.click()
      URL.revokeObjectURL(url)
      return backup.agents.length
    },
  })
}

export function useRestoreAgents() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (backup: AgentsBackup) => unwrap(api.POST("/backup/agents", { body: backup })),
    // New agents and their DMs also arrive over the WebSocket; refetch in case it's down.
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.agents })
      void queryClient.invalidateQueries({ queryKey: queryKeys.chats })
    },
  })
}

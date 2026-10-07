import type { ConnectorType, FormField, FormValues } from "@/features/connectors"
import type { Schemas } from "@/lib/api-client"

import type { Prompt } from "./logic"

export type PromptConnection = Schemas["PromptConnection"]
export type ConnectPromptRequest = Schemas["ConnectPromptRequest"]

/** Connect prompts' options are always ["Connect", "Decline"]; Decline is answered with [1]. */
export const DECLINE_CONNECT = { selected: [1] }

/**
 * The secrets the card asks for: one input per credential field. Credentials are always
 * hidden by default (Trust), even if the type doesn't flag one as secret.
 */
export function connectFields(type: ConnectorType): FormField[] {
  return type.credentialFields.map((field) => ({
    ...field,
    group: "credentials",
    input: "secret",
    saved: false,
    required: !field.optional,
  }))
}

/** The connect request: trimmed credential values, leaving out empty optional ones. */
export function connectRequestBody(
  fields: FormField[],
  values: FormValues,
  name?: string,
): ConnectPromptRequest {
  const credentials: Record<string, string> = {}
  for (const field of fields) {
    const value = (values[`${field.group}.${field.key}`] ?? "").trim()
    if (value) credentials[field.key] = value
  }
  const trimmedName = name?.trim()
  return { credentials, ...(trimmedName ? { name: trimmedName } : {}) }
}

export type ConfigRow = { key: string; label: string; value: string }

/** The agent's non-secret settings, labeled from the type's config fields (falling back to the key). */
export function configRows(
  connection: PromptConnection,
  type: ConnectorType | undefined,
): ConfigRow[] {
  return Object.entries(connection.config)
    .filter(([, value]) => value !== "")
    .map(([key, value]) => ({
      key,
      value,
      label: type?.configFields.find((f) => f.key === key)?.label ?? key,
    }))
}

/** How an answered connect prompt came out. */
export function connectOutcome(prompt: Prompt): "connected" | "declined" | null {
  if (prompt.kind !== "connect" || prompt.status !== "answered") return null
  return prompt.connection?.accountId ? "connected" : "declined"
}

/** "Access: barn, Tracker" (unknown agents are skipped). */
export function accessLabel(agentIds: string[], names: Map<string, string>): string {
  const known = agentIds.flatMap((id) => {
    const name = names.get(id)
    return name ? [name] : []
  })
  return known.length > 0 ? `Access: ${known.join(", ")}` : "Access: no agents"
}

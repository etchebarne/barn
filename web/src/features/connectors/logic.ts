import type { Agent, Schemas } from "@/lib/api-client"

export type ConnectorType = Schemas["ConnectorType"]
export type ConnectorField = Schemas["ConnectorField"]
export type Connector = Schemas["Connector"]

export type FormField = ConnectorField & {
  group: "credentials" | "config"
  /** Password input with a show toggle. */
  input: "secret" | "text"
  /** Needs a value (when creating; saved credentials may stay empty when editing). */
  required: boolean
  /** A value is already saved for this credential (editing only). */
  saved: boolean
}

export type FormValues = Record<string, string>

/** Field key in the form, namespaced so a credential and a config key can't collide. */
export function fieldId(field: Pick<FormField, "group" | "key">): string {
  return `${field.group}.${field.key}`
}

/**
 * The form for a connector type: required config (what to connect to, like a server URL or
 * command), then credentials, then optional config. When editing, credentials with a saved
 * value aren't required (leaving them empty keeps the saved one).
 */
export function buildFormFields(type: ConnectorType, connector?: Connector): FormField[] {
  const saved = new Set(connector?.credentialsSet ?? [])
  const credentials: FormField[] = type.credentialFields.map((field) => ({
    ...field,
    group: "credentials",
    input: field.secret ? "secret" : "text",
    saved: saved.has(field.key),
    required: !field.optional && !saved.has(field.key),
  }))
  const config: FormField[] = type.configFields.map((field) => ({
    ...field,
    group: "config",
    input: field.secret ? "secret" : "text",
    saved: false,
    // Config fields that already have a value are pre-filled, so the rule is the same.
    required: !field.optional,
  }))
  return [...config.filter((f) => f.required), ...credentials, ...config.filter((f) => !f.required)]
}

/** Starting values: empty credentials (never returned), current config when editing. */
export function initialValues(fields: FormField[], connector?: Connector): FormValues {
  const values: FormValues = {}
  for (const field of fields) {
    values[fieldId(field)] = field.group === "config" ? (connector?.config[field.key] ?? "") : ""
  }
  return values
}

export function validateForm(fields: FormField[], values: FormValues): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const field of fields) {
    if (field.required && !values[fieldId(field)]?.trim()) {
      errors[fieldId(field)] = `${field.label} is required.`
    }
  }
  return errors
}

function collect(
  fields: FormField[],
  values: FormValues,
  group: FormField["group"],
  skipEmpty: boolean,
) {
  const out: Record<string, string> = {}
  for (const field of fields) {
    if (field.group !== group) continue
    const value = (values[fieldId(field)] ?? "").trim()
    if (skipEmpty && !value) continue
    out[field.key] = value
  }
  return out
}

export function buildCreateRequest(
  type: ConnectorType,
  fields: FormField[],
  values: FormValues,
  name: string,
  agentIds: string[],
): Schemas["CreateConnectorRequest"] {
  const config = collect(fields, values, "config", true)
  return {
    type: type.type,
    name: name.trim() || type.name,
    credentials: collect(fields, values, "credentials", true),
    ...(Object.keys(config).length > 0 ? { config } : {}),
    agentIds,
  }
}

/** An update: only credentials the user typed (empty keeps the saved value), all config. */
export function buildUpdateRequest(
  fields: FormField[],
  values: FormValues,
): Schemas["UpdateConnectorRequest"] {
  const credentials = collect(fields, values, "credentials", true)
  return {
    ...(Object.keys(credentials).length > 0 ? { credentials } : {}),
    config: collect(fields, values, "config", false),
  }
}

/** Grants with one agent turned on or off (sorted, unique). */
export function toggleGrant(agentIds: string[], agentId: string, on: boolean): string[] {
  const next = new Set(agentIds)
  if (on) next.add(agentId)
  else next.delete(agentId)
  return [...next].toSorted()
}

/** "Used by: barn, Tracker" or "No agents yet". */
export function usedByLabel(agentIds: string[], agents: Map<string, Agent>): string {
  const names = agentIds.flatMap((id) => {
    const name = agents.get(id)?.name
    return name ? [name] : []
  })
  return names.length > 0 ? `Used by: ${names.join(", ")}` : "No agents yet"
}

/** The generic webhook type's URL embeds a secret token. */
export function webhookUrlIsSecret(type: string): boolean {
  return type === "webhook"
}

/**
 * Masks the token in a webhook URL: everything after the last "/" except its first 4
 * characters, plus any query string. "https://h/hooks/webhook/abcd••••••••".
 */
export function maskWebhookUrl(url: string): string {
  const [path = "", query] = url.split("?", 2)
  const slash = path.lastIndexOf("/")
  const token = path.slice(slash + 1)
  const maskedPath =
    token.length > 4 ? `${path.slice(0, slash + 1)}${token.slice(0, 4)}${"•".repeat(8)}` : path
  return query === undefined ? maskedPath : `${maskedPath}?${"•".repeat(8)}`
}

import type { Connector, ConnectorType } from "./logic"
import type { CatalogApp } from "./signin"

/** Types you configure yourself (any server, any webhook): offered under Add, not as apps. */
export const CUSTOM_TYPES = ["mcp", "mcp_local", "webhook"]

/** Where the add flow starts: an app's sign-in, or a type's form. */
export type AddStart = { kind: "app" | "type"; id: string }

/** One card on Discover: a sign-in app from the catalog, or a connector type. */
export type DiscoverEntry = AddStart & {
  /** Key for `ConnectorIcon`. */
  logo: string
  name: string
  description: string
  /** How it connects, shown under the description. */
  method: string
}

/** Discover's apps: sign-in apps first, then the rest (API key), custom types left out. */
export function discoverEntries(
  catalog: CatalogApp[] | undefined,
  types: ConnectorType[] | undefined,
): DiscoverEntry[] {
  const apps: DiscoverEntry[] = (catalog ?? []).map((app) => ({
    kind: "app",
    id: app.id,
    logo: app.id,
    name: app.name,
    description: app.description,
    method: "Sign in",
  }))
  // An app that has both (Linear) gets one card: sign-in, with the key as an option inside.
  const signInNames = new Set(apps.map((a) => a.name.toLowerCase()))
  const keyed: DiscoverEntry[] = (types ?? [])
    .filter((t) => !CUSTOM_TYPES.includes(t.type) && !signInNames.has(t.name.toLowerCase()))
    .map((t) => ({
      kind: "type",
      id: t.type,
      logo: t.type,
      name: t.name,
      description: t.description,
      method: "API key",
    }))
  return [...apps, ...keyed].toSorted((a, b) => a.name.localeCompare(b.name))
}

/** The custom types, for the Add menu. */
export function customTypes(types: ConnectorType[] | undefined): ConnectorType[] {
  return (types ?? []).filter((t) => CUSTOM_TYPES.includes(t.type))
}

/**
 * The catalog app a connection was made from (signing in makes an MCP connection to the app's
 * server), so it can show the app's logo and name.
 */
export function connectionApp(
  connector: Connector,
  catalog: CatalogApp[] | undefined,
): CatalogApp | undefined {
  if (connector.type !== "mcp") return undefined
  const url = connector.config.url
  return url ? catalog?.find((app) => app.url === url) : undefined
}

/** Whether there's already a connection for this card. */
export function isConnected(
  entry: AddStart,
  connectors: Connector[] | undefined,
  catalog: CatalogApp[] | undefined,
): boolean {
  return (connectors ?? []).some((c) =>
    entry.kind === "app"
      ? connectionApp(c, catalog)?.id === entry.id || c.type === entry.id
      : c.type === entry.id,
  )
}

/** The URL selection that opens the add flow at a card: "new:app:notion", "new:type:slack". */
export function addSelection(start: AddStart): string {
  return `new:${start.kind}:${start.id}`
}

/** "new" (pick an app) or "new:<kind>:<id>" (start there); null for anything else. */
export function parseAddSelection(
  selection: string | undefined,
): { start: AddStart | null } | null {
  if (selection === "new") return { start: null }
  const match = /^new:(app|type):(.+)$/.exec(selection ?? "")
  if (!match) return null
  return { start: { kind: match[1] === "app" ? "app" : "type", id: match[2] ?? "" } }
}

/** Case-insensitive match of a search query against any of the texts. */
export function matchesQuery(query: string, ...texts: string[]): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return texts.some((t) => t.toLowerCase().includes(q))
}

import { describe, expect, it } from "vitest"

import {
  addSelection,
  connectionApp,
  discoverEntries,
  isConnected,
  matchesQuery,
  parseAddSelection,
} from "./discover"
import type { Connector, ConnectorType } from "./logic"
import type { CatalogApp } from "./signin"

const catalog: CatalogApp[] = [
  { id: "notion", name: "Notion", description: "Pages", url: "https://mcp.notion.com/mcp" },
  { id: "linear", name: "Linear", description: "Issues", url: "https://mcp.linear.app/mcp" },
]
const type = (t: string, name: string) =>
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- only the fields used here
  ({ type: t, name, description: `${name} things` }) as unknown as ConnectorType
const types = [
  type("slack", "Slack"),
  type("linear", "Linear"),
  type("mcp", "MCP server"),
  type("webhook", "Webhook"),
]
const connector = (t: string, config: Record<string, string> = {}) =>
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- only the fields used here
  ({ id: t, type: t, name: t, config, agentIds: [] }) as unknown as Connector

describe("discoverEntries", () => {
  it("lists sign-in apps and key types once each, alphabetically, without custom types", () => {
    expect(discoverEntries(catalog, types).map((e) => `${e.kind}:${e.id}`)).toEqual([
      "app:linear",
      "app:notion",
      "type:slack",
    ])
  })
})

describe("connections", () => {
  it("finds the app a signed-in MCP connection came from", () => {
    const signedIn = connector("mcp", { url: "https://mcp.notion.com/mcp" })
    expect(connectionApp(signedIn, catalog)?.id).toBe("notion")
    expect(connectionApp(connector("mcp", { url: "https://x" }), catalog)).toBeUndefined()
  })

  it("marks a card connected by app or by type", () => {
    const mine = [connector("mcp", { url: "https://mcp.notion.com/mcp" }), connector("linear")]
    expect(isConnected({ kind: "app", id: "notion" }, mine, catalog)).toBe(true)
    // Linear's card covers both its sign-in and its API key.
    expect(isConnected({ kind: "app", id: "linear" }, mine, catalog)).toBe(true)
    expect(isConnected({ kind: "type", id: "slack" }, mine, catalog)).toBe(false)
  })
})

describe("add selection", () => {
  it("round-trips through the URL", () => {
    expect(parseAddSelection(addSelection({ kind: "type", id: "slack" }))).toEqual({
      start: { kind: "type", id: "slack" },
    })
    expect(parseAddSelection("new")).toEqual({ start: null })
    expect(parseAddSelection("01ABC")).toBeNull()
    expect(parseAddSelection(undefined)).toBeNull()
  })
})

describe("matchesQuery", () => {
  it("matches any text, ignoring case; empty matches all", () => {
    expect(matchesQuery("", "x")).toBe(true)
    expect(matchesQuery("SLA", "Slack")).toBe(true)
    expect(matchesQuery("git", "Slack", "chat")).toBe(false)
  })
})

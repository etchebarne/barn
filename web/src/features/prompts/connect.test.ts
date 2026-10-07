import { describe, expect, it } from "vitest"

import type { ConnectorType } from "@/features/connectors"

import {
  accessLabel,
  configRows,
  connectFields,
  connectOutcome,
  connectRequestBody,
} from "./connect"
import type { Prompt } from "./logic"

const notion: ConnectorType = {
  type: "mcp",
  name: "MCP server",
  description: "Any MCP server",
  credentialFields: [
    { key: "token", label: "Access token", secret: true, optional: false, events: false },
    { key: "extra", label: "Extra header", secret: false, optional: true, events: false },
  ],
  configFields: [
    { key: "url", label: "Server URL", secret: false, optional: false, events: false },
  ],
  signals: [],
  webhooks: false,
  setup: { steps: [], eventSteps: [] },
}

function prompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    kind: "connect",
    question: "Connect Notion (MCP server)?\nTo search your docs.",
    options: [{ label: "Connect" }, { label: "Decline" }],
    allowOther: false,
    status: "pending",
    answer: null,
    connection: {
      type: "mcp",
      name: "Notion",
      config: { url: "https://mcp.notion.com/mcp", region: "" },
      agentIds: ["a1", "a2"],
      signIn: false,
      accountId: null,
    },
    ...overrides,
  }
}

describe("connect prompt logic", () => {
  it("leaves out event-only credentials", () => {
    const withEvents: ConnectorType = {
      ...notion,
      credentialFields: [
        ...notion.credentialFields,
        { key: "signing", label: "Signing secret", secret: true, optional: true, events: true },
      ],
    }
    expect(connectFields(withEvents).map((f) => f.key)).toEqual(["token", "extra"])
  })

  it("asks for every credential, always hidden", () => {
    expect(connectFields(notion).map((f) => [f.key, f.input, f.required])).toEqual([
      ["token", "secret", true],
      ["extra", "secret", false],
    ])
  })

  it("sends trimmed credentials and leaves out empty optional ones", () => {
    const fields = connectFields(notion)
    expect(
      connectRequestBody(fields, { "credentials.token": "  sk-1 ", "credentials.extra": "" }),
    ).toEqual({ credentials: { token: "sk-1" } })
    expect(connectRequestBody(fields, { "credentials.token": "sk-1" }, " Docs ")).toEqual({
      credentials: { token: "sk-1" },
      name: "Docs",
    })
  })

  it("labels config values from the type, falling back to the key, skipping empty ones", () => {
    const connection = prompt().connection
    if (!connection) throw new Error("fixture")
    expect(configRows(connection, notion)).toEqual([
      { key: "url", label: "Server URL", value: "https://mcp.notion.com/mcp" },
    ])
    expect(configRows({ ...connection, config: { team: "x" } }, notion)[0]?.label).toBe("team")
  })

  it("reads the outcome from the connection", () => {
    expect(connectOutcome(prompt())).toBeNull()
    expect(connectOutcome(prompt({ status: "answered", answer: { selected: [1] } }))).toBe(
      "declined",
    )
    const connection = { ...prompt().connection!, accountId: "k9" }
    expect(connectOutcome(prompt({ status: "answered", connection }))).toBe("connected")
  })

  it("names who gets access", () => {
    expect(
      accessLabel(
        ["a1", "a2", "gone"],
        new Map([
          ["a1", "openbot"],
          ["a2", "Tracker"],
        ]),
      ),
    ).toBe("Access: openbot, Tracker")
  })
})

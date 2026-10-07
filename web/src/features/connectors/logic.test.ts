import { describe, expect, it } from "vitest"

import { makeAgent } from "@/test/fixtures"

import {
  buildCreateRequest,
  buildFormFields,
  buildUpdateRequest,
  fieldId,
  initialValues,
  maskWebhookUrl,
  toggleGrant,
  usedByLabel,
  validateForm,
  type Connector,
  type ConnectorType,
} from "./logic"

const slack: ConnectorType = {
  type: "slack",
  name: "Slack",
  description: "Read and post in your workspace",
  credentialFields: [
    {
      key: "botToken",
      label: "Bot token",
      help: "Starts with xoxb-",
      secret: true,
      optional: false,
    },
    { key: "appToken", label: "App token", secret: true, optional: true },
  ],
  configFields: [{ key: "team", label: "Team", secret: false, optional: false }],
  signals: [],
  webhooks: false,
}

const saved: Connector = {
  id: "c1",
  type: "slack",
  name: "Slack (work)",
  config: { team: "acme" },
  credentialsSet: ["botToken"],
  agentIds: ["a1"],
  webhookUrl: null,
  createdAt: "2026-10-01T00:00:00Z",
}

describe("form generation", () => {
  it("makes secret fields password inputs and marks optional ones not required", () => {
    const fields = buildFormFields(slack)
    expect(fields.map((f) => [fieldId(f), f.input, f.required])).toEqual([
      ["credentials.botToken", "secret", true],
      ["credentials.appToken", "secret", false],
      ["config.team", "text", true],
    ])
    expect(fields[0]?.help).toBe("Starts with xoxb-")
  })

  it("doesn't require saved credentials when editing, and pre-fills config", () => {
    const fields = buildFormFields(slack, saved)
    expect(fields.find((f) => f.key === "botToken")).toMatchObject({ saved: true, required: false })
    expect(initialValues(fields, saved)).toEqual({
      "credentials.botToken": "",
      "credentials.appToken": "",
      "config.team": "acme",
    })
  })

  it("validates required fields", () => {
    const fields = buildFormFields(slack)
    expect(validateForm(fields, initialValues(fields))).toEqual({
      "credentials.botToken": "Bot token is required.",
      "config.team": "Team is required.",
    })
  })

  it("builds a create request, skipping empty optional values", () => {
    const fields = buildFormFields(slack)
    const values = {
      "credentials.botToken": " xoxb-1 ",
      "credentials.appToken": "",
      "config.team": "acme",
    }
    expect(buildCreateRequest(slack, fields, values, "  ", ["a1"])).toEqual({
      type: "slack",
      name: "Slack",
      credentials: { botToken: "xoxb-1" },
      config: { team: "acme" },
      agentIds: ["a1"],
    })
  })

  it("keeps saved credentials on update by leaving them out", () => {
    const fields = buildFormFields(slack, saved)
    expect(buildUpdateRequest(fields, initialValues(fields, saved))).toEqual({
      config: { team: "acme" },
    })
    expect(
      buildUpdateRequest(fields, {
        ...initialValues(fields, saved),
        "credentials.botToken": "new",
      }),
    ).toEqual({ credentials: { botToken: "new" }, config: { team: "acme" } })
  })
})

describe("maskWebhookUrl", () => {
  it("hides the token after the last slash and any query", () => {
    expect(maskWebhookUrl("https://barn.example/hooks/webhook/s3cr3tt0k3n")).toBe(
      "https://barn.example/hooks/webhook/s3cr••••••••",
    )
    expect(maskWebhookUrl("https://barn.example/hooks/x/abcdef?token=zzz")).toBe(
      "https://barn.example/hooks/x/abcd••••••••?••••••••",
    )
    expect(maskWebhookUrl("https://h/ab")).toBe("https://h/ab")
  })
})

describe("grants", () => {
  it("toggles an agent on or off, sorted and unique", () => {
    expect(toggleGrant(["b"], "a", true)).toEqual(["a", "b"])
    expect(toggleGrant(["a", "b"], "a", true)).toEqual(["a", "b"])
    expect(toggleGrant(["a", "b"], "a", false)).toEqual(["b"])
    expect(toggleGrant([], "a", false)).toEqual([])
  })

  it("labels who uses a connection", () => {
    const agents = new Map([
      ["a1", makeAgent({ id: "a1", name: "barn" })],
      ["a2", makeAgent({ id: "a2", name: "Tracker" })],
    ])
    expect(usedByLabel(["a1", "a2"], agents)).toBe("Used by: barn, Tracker")
    expect(usedByLabel([], agents)).toBe("No agents yet")
    expect(usedByLabel(["gone"], agents)).toBe("No agents yet")
  })
})

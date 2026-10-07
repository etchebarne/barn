import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ConnectorFields } from "./connector-fields"
import { buildFormFields, initialValues, type Connector, type ConnectorType } from "./logic"

const github: ConnectorType = {
  type: "github",
  name: "GitHub",
  description: "Repos, issues and PRs",
  credentialFields: [
    {
      key: "token",
      label: "Access token",
      help: "A fine-grained token",
      secret: true,
      optional: false,
      events: false,
    },
  ],
  configFields: [
    { key: "org", label: "Organization", secret: false, optional: true, events: false },
  ],
  signals: [],
  webhooks: true,
  setup: { steps: [], eventSteps: [] },
}

function renderFields(connector?: Connector) {
  const fields = buildFormFields(github, connector)
  render(
    <ConnectorFields
      idPrefix="t"
      fields={fields}
      values={initialValues(fields, connector)}
      errors={{}}
      onChange={vi.fn<(id: string, value: string) => void>()}
    />,
  )
}

describe("ConnectorFields", () => {
  it("hides secrets by default with a show toggle, and shows help and optional marks", async () => {
    renderFields()
    const token = screen.getByLabelText("Access token")
    expect(token).toHaveAttribute("type", "password")
    expect(screen.getByText("A fine-grained token")).toBeVisible()
    expect(screen.getByLabelText(/Organization/)).toHaveAttribute("type", "text")
    expect(screen.getByText("(optional)")).toBeVisible()

    await userEvent.click(screen.getByRole("button", { name: "Show" }))
    expect(token).toHaveAttribute("type", "text")
  })

  it("marks saved credentials", () => {
    renderFields({
      id: "c1",
      type: "github",
      name: "GitHub",
      config: {},
      credentialsSet: ["token"],
      agentIds: [],
      webhookUrl: null,
      webhookSecret: null,
      signIn: null,
      createdAt: "2026-10-01T00:00:00Z",
    })
    expect(screen.getByText("Saved")).toBeVisible()
    expect(screen.getByLabelText(/Access token/)).toHaveAttribute(
      "placeholder",
      "Saved. Leave empty to keep it",
    )
  })
})

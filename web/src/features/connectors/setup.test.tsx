import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import { Connected } from "./add-connection"
import { ConnectorDetail } from "./connector-detail"
import {
  buildFormFields,
  buildUpdateRequest,
  connectFormFields,
  eventFormFields,
  hasEventsSection,
  initialValues,
  type Connector,
  type ConnectorType,
} from "./logic"
import { SetupSteps } from "./setup-steps"
import { WebhookSecret } from "./webhook-info"

const github: ConnectorType = {
  type: "github",
  name: "GitHub",
  description: "Repos, issues and PRs",
  credentialFields: [
    {
      key: "token",
      label: "Access token",
      help: "Starts with github_pat_.",
      secret: true,
      optional: false,
      events: false,
    },
    { key: "webhookSecret", label: "Webhook secret", secret: true, optional: true, events: true },
  ],
  configFields: [],
  signals: [{ type: "github.pull_request", description: "A PR is opened or updated", fields: [] }],
  webhooks: true,
  setup: {
    steps: [
      {
        text: "Create a fine-grained token with access to your repos.",
        link: {
          label: "Open GitHub",
          url: "https://github.com/settings/personal-access-tokens/new",
        },
        copy: null,
      },
      {
        text: "Or create barn's GitHub app from this manifest.",
        link: null,
        copy: { label: "App manifest", text: '{"name":"barn","very":"long"}' },
      },
    ],
    eventSteps: [
      {
        text: "In the repo's Settings → Webhooks, add a webhook with this URL and secret.",
        link: null,
        copy: null,
      },
    ],
  },
}

const connector: Connector = {
  id: "k1",
  type: "github",
  name: "GitHub (work)",
  config: {},
  credentialsSet: ["token"],
  agentIds: [],
  webhookUrl: "https://barn.example/hooks/github/k1",
  webhookSecret: "whsec_7f3a9c1e5b",
  createdAt: "2026-10-01T00:00:00Z",
}

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>{ui}</TooltipProvider>
    </QueryClientProvider>,
  )
}

describe("SetupSteps", () => {
  it("numbers the steps, opens links in a new tab, and copies without showing the text", () => {
    wrap(<SetupSteps steps={github.setup.steps} label="How to connect GitHub" />)
    const list = screen.getByRole("list", { name: "How to connect GitHub" })
    expect(within(list).getAllByRole("listitem")).toHaveLength(2)

    const link = screen.getByRole("link", { name: /Open GitHub/ })
    expect(link).toHaveAttribute("href", "https://github.com/settings/personal-access-tokens/new")
    expect(link).toHaveAttribute("target", "_blank")
    expect(link).toHaveAttribute("rel", "noopener noreferrer")

    expect(screen.getByRole("button", { name: "Copy App manifest" })).toBeVisible()
    expect(screen.queryByText(/"very":"long"/)).not.toBeInTheDocument()
  })

  it("renders nothing without steps", () => {
    const { container } = wrap(<SetupSteps steps={[]} label="none" />)
    expect(container.querySelector("ol")).toBeNull()
  })
})

describe("events fields", () => {
  it("are left out of the connect form and kept for the events form", () => {
    const fields = buildFormFields(github)
    expect(connectFormFields(fields).map((f) => f.key)).toEqual(["token"])
    expect(eventFormFields(fields).map((f) => f.key)).toEqual(["webhookSecret"])
  })

  it("update without touching config when the form has no config fields", () => {
    const fields = eventFormFields(buildFormFields(github, connector))
    expect(
      buildUpdateRequest(fields, {
        ...initialValues(fields, connector),
        "credentials.webhookSecret": "s",
      }),
    ).toEqual({ credentials: { webhookSecret: "s" } })
  })

  it("decide when a connection gets a Receive events section", () => {
    expect(hasEventsSection(github, connector)).toBe(true)
    const plain = {
      ...github,
      webhooks: false,
      setup: { steps: [], eventSteps: [] },
      credentialFields: [],
    }
    expect(hasEventsSection(plain, { ...connector, webhookUrl: null })).toBe(false)
  })

  it("show up in the connection's Receive events section, with the saved badge rules", () => {
    wrap(
      <ConnectorDetail connector={connector} type={github} onDisconnected={vi.fn<() => void>()} />,
    )
    const events = screen.getByRole("region", { name: "Receive events" })
    expect(within(events).getByLabelText(/^Webhook secret/, { selector: "input" })).toHaveAttribute(
      "type",
      "password",
    )
    expect(within(events).getByText(/add a webhook with this URL and secret/)).toBeVisible()
    expect(within(events).getByText("Events agents can react to")).toBeVisible()
    // The main form only has the connect credentials.
    expect(screen.getAllByLabelText(/Access token/)).toHaveLength(1)
    expect(within(events).queryByLabelText(/Access token/)).not.toBeInTheDocument()
  })
})

describe("WebhookSecret", () => {
  it("is masked until revealed, and copyable", async () => {
    wrap(<WebhookSecret connector={connector} type={github} />)
    expect(screen.queryByText("whsec_7f3a9c1e5b")).not.toBeInTheDocument()
    expect(screen.getByTestId("webhook-secret")).toHaveTextContent("•")
    expect(screen.getByRole("button", { name: "Copy webhook secret" })).toBeVisible()

    await userEvent.click(screen.getByRole("button", { name: "Show webhook secret" }))
    expect(screen.getByText("whsec_7f3a9c1e5b")).toBeVisible()
  })

  it("renders nothing without a secret", () => {
    const { container } = wrap(
      <WebhookSecret connector={{ ...connector, webhookSecret: null }} type={github} />,
    )
    expect(container.textContent).toBe("")
  })
})

describe("done screen", () => {
  it("points to the events setup when the type has event steps", () => {
    wrap(
      <Connected
        connector={connector}
        type={github}
        onDone={vi.fn<() => void>()}
        onOpen={vi.fn<() => void>()}
      />,
    )
    expect(
      screen.getByText(
        /To let agents react to GitHub events, follow the steps under Receive events/,
      ),
    ).toBeVisible()
    expect(screen.getByText("Webhook secret")).toBeVisible()
  })

  it("has no hint without event steps", () => {
    wrap(
      <Connected
        connector={{ ...connector, webhookUrl: null, webhookSecret: null }}
        type={{ ...github, setup: { ...github.setup, eventSteps: [] } }}
        onDone={vi.fn<() => void>()}
        onOpen={vi.fn<() => void>()}
      />,
    )
    expect(screen.queryByText(/To let agents react/)).not.toBeInTheDocument()
  })
})

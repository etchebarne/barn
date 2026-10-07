import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import type { ConnectorType } from "@/features/connectors"
import { ApiError } from "@/lib/api-client"

import type { ConnectPromptRequest } from "./connect"
import { ConnectCard } from "./connect-card"
import type { Prompt } from "./logic"

const mcp: ConnectorType = {
  type: "mcp",
  name: "MCP server",
  description: "Any MCP server",
  credentialFields: [
    {
      key: "token",
      label: "Access token",
      help: "From Notion's integrations page",
      secret: true,
      optional: false,
    },
    { key: "header", label: "Extra header", secret: false, optional: true },
  ],
  configFields: [{ key: "url", label: "Server URL", secret: false, optional: false }],
  signals: [],
  webhooks: false,
}

const webhookType: ConnectorType = {
  ...mcp,
  type: "webhook",
  name: "Webhook",
  credentialFields: [],
}

function prompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    kind: "connect",
    question: "Connect Notion (MCP server)?\nSo I can search your docs.",
    options: [{ label: "Connect" }, { label: "Decline" }],
    allowOther: false,
    status: "pending",
    answer: null,
    connection: {
      type: "mcp",
      name: "Notion",
      config: { url: "https://mcp.notion.com/mcp" },
      agentIds: ["a1"],
      accountId: null,
    },
    ...overrides,
  }
}

async function renderInRouter(ui: ReactNode) {
  const rootRoute = createRootRoute({ component: () => <TooltipProvider>{ui}</TooltipProvider> })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router, not the app's registered one
  render(<RouterProvider router={router as never} />)
  await screen.findByText(/Connect Notion/)
}

function card(overrides: Partial<Parameters<typeof ConnectCard>[0]> = {}) {
  return (
    <ConnectCard
      prompt={prompt()}
      type={mcp}
      typesLoading={false}
      agentNames={new Map([["a1", "barn"]])}
      isLatest
      disabled={false}
      onConnect={vi.fn<(body: ConnectPromptRequest) => Promise<void>>().mockResolvedValue()}
      onDecline={vi.fn<() => void>()}
      onDismiss={vi.fn<() => void>()}
      {...overrides}
    />
  )
}

describe("ConnectCard", () => {
  it("shows what's being connected, the settings, access, and hidden secret inputs", async () => {
    await renderInRouter(card())
    expect(screen.getByText("Notion")).toBeVisible()
    expect(screen.getByText("· MCP server")).toBeVisible()
    expect(screen.getByText("Server URL")).toBeVisible()
    expect(screen.getByText("https://mcp.notion.com/mcp")).toBeVisible()
    expect(screen.getByText("Access: barn")).toBeVisible()

    const token = screen.getByLabelText("Access token")
    expect(token).toHaveAttribute("type", "password")
    expect(screen.getByLabelText(/Extra header/)).toHaveAttribute("type", "password")
    expect(screen.getByText("(optional)")).toBeVisible()
    expect(screen.getByText("From Notion's integrations page")).toBeVisible()

    const [show] = screen.getAllByRole("button", { name: "Show" })
    await userEvent.click(show)
    expect(token).toHaveAttribute("type", "text")
  })

  it("shows only the buttons when the type has no credentials", async () => {
    await renderInRouter(card({ type: webhookType }))
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument()
    expect(document.querySelector("input")).toBeNull()
    expect(screen.getByRole("button", { name: "Connect" })).toBeVisible()
    expect(screen.getByRole("button", { name: "Decline" })).toBeVisible()
  })

  it("sends the credentials and clears them after connecting", async () => {
    const onConnect = vi.fn<(body: ConnectPromptRequest) => Promise<void>>().mockResolvedValue()
    await renderInRouter(card({ onConnect }))

    await userEvent.type(screen.getByLabelText("Access token"), " ntn_secret ")
    await userEvent.click(screen.getByRole("button", { name: "Connect" }))

    expect(onConnect).toHaveBeenCalledWith({ credentials: { token: "ntn_secret" } })
    expect(screen.getByLabelText("Access token")).toHaveValue("")
  })

  it("requires non-optional credentials before connecting", async () => {
    const onConnect = vi.fn<(body: ConnectPromptRequest) => Promise<void>>().mockResolvedValue()
    await renderInRouter(card({ onConnect }))
    await userEvent.click(screen.getByRole("button", { name: "Connect" }))
    expect(onConnect).not.toHaveBeenCalled()
    expect(screen.getByText("Access token is required.")).toBeVisible()
  })

  it("shows the app's rejection inline and keeps the values for a retry", async () => {
    const onConnect = vi
      .fn<(body: ConnectPromptRequest) => Promise<void>>()
      .mockRejectedValue(new ApiError(400, "Notion rejected the token (401 Unauthorized)."))
    await renderInRouter(card({ onConnect }))

    await userEvent.type(screen.getByLabelText("Access token"), "wrong")
    await userEvent.click(screen.getByRole("button", { name: "Connect" }))

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Notion rejected the token (401 Unauthorized).",
    )
    expect(screen.getByLabelText("Access token")).toHaveValue("wrong")
    expect(screen.getByRole("button", { name: "Connect" })).toBeEnabled()
  })

  it("declines with the Decline button", async () => {
    const onDecline = vi.fn<() => void>()
    await renderInRouter(card({ onDecline }))
    await userEvent.click(screen.getByRole("button", { name: "Decline" }))
    expect(onDecline).toHaveBeenCalled()
  })

  it("shows Connected with a link to the connection once answered", async () => {
    const connection = { ...prompt().connection!, accountId: "k9" }
    await renderInRouter(
      card({ prompt: prompt({ status: "answered", answer: { selected: [0] }, connection }) }),
    )
    expect(screen.getByText("Connected")).toBeVisible()
    expect(screen.getByRole("link", { name: "View connection" })).toHaveAttribute(
      "href",
      "/settings?connector=k9",
    )
    expect(document.querySelector("input")).toBeNull()
  })

  it("shows Declined when answered without a connection", async () => {
    await renderInRouter(
      card({ prompt: prompt({ status: "answered", answer: { selected: [1] } }) }),
    )
    expect(screen.getByText("Declined")).toBeVisible()
    expect(screen.queryByRole("button", { name: "Connect" })).not.toBeInTheDocument()
  })
})

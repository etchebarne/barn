import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { makeAgent } from "@/test/fixtures"

import type { StandingApproval } from "./api"
import { StandingApprovalsSection } from "./standing-approvals-section"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(text: string) => void>(), error: vi.fn<(text: string) => void>() },
}))

const slack: StandingApproval = {
  id: "sa1",
  label: "Slack message · Work Slack, when Channel is #bot-ception",
  createdAt: "2026-10-07T00:00:00Z",
}
const github: StandingApproval = {
  id: "sa2",
  label: "New issue · GitHub, when Repo is acme/web",
  createdAt: "2026-10-07T00:00:00Z",
}

function serve(list: StandingApproval[]) {
  const response = new Response(null, { status: 200 })
  vi.spyOn(api, "GET").mockImplementation(() =>
    Promise.resolve({ data: list, error: undefined, response }),
  )
}

function renderSection(agent = makeAgent({ name: "barn" })) {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <StandingApprovalsSection agent={agent} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("Allowed without asking", () => {
  it("lists standing approvals", async () => {
    serve([slack, github])
    renderSection()
    expect(await screen.findByText(slack.label)).toBeVisible()
    expect(screen.getByText(github.label)).toBeVisible()
    expect(api.GET).toHaveBeenCalledWith("/agents/{agentId}/approvals", {
      params: { path: { agentId: "agent-1" } },
    })
  })

  it("removes one and updates the list", async () => {
    serve([slack, github])
    const del = vi.spyOn(api, "DELETE").mockImplementation(() =>
      Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(null, { status: 204 }),
      }),
    )
    renderSection()
    await userEvent.click(await screen.findByRole("button", { name: `Remove: ${slack.label}` }))
    expect(del).toHaveBeenCalledWith("/agents/{agentId}/approvals/{approvalId}", {
      params: { path: { agentId: "agent-1", approvalId: "sa1" } },
    })
    await waitFor(() => expect(screen.queryByText(slack.label)).not.toBeInTheDocument())
    expect(screen.getByText(github.label)).toBeVisible()
  })

  it("explains how to add some when empty", async () => {
    serve([])
    renderSection()
    expect(
      await screen.findByText(
        "Nothing yet. When barn asks for approval, choose Always allow, or tell it what it can do without asking.",
      ),
    ).toBeVisible()
  })

  it("notes that trusted mode already skips approvals", async () => {
    serve([slack])
    renderSection(makeAgent({ name: "barn", trustMode: "trusted" }))
    expect(await screen.findByText(slack.label)).toBeVisible()
    expect(
      screen.getByText(/Trusted mode is on, so barn already skips all approvals/),
    ).toBeVisible()
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { makeAgent, makeUsageReport, makeUsageTotals } from "@/test/fixtures"

import { AgentUsageSection } from "./agent-usage-section"
import type { UsageReport } from "./api"

const agent = makeAgent({ id: "digest", name: "Weekly Digest" })

function mockUsage(report: UsageReport) {
  return vi
    .spyOn(api, "GET")
    .mockImplementation(() =>
      Promise.resolve({ data: report, error: undefined, response: new Response(null) }),
    )
}

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <AgentUsageSection agent={agent} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("agent usage section", () => {
  it("shows the context meter, today's and the week's totals, days and purposes", async () => {
    const today = makeUsageTotals({
      calls: 4,
      promptTokens: 12_300,
      cachedTokens: 6150,
      completionTokens: 900,
    })
    const week = makeUsageTotals({
      calls: 31,
      promptTokens: 3_400_000,
      cachedTokens: 850_000,
      completionTokens: 45_600,
    })
    const report = makeUsageReport({
      today,
      total: week,
      context: { tokens: 23_400, window: 1_000_000, compactAt: 64_000 },
      byPurpose: [
        { purpose: "compaction", ...makeUsageTotals({ calls: 1, promptTokens: 400_000 }) },
        { purpose: "turn", ...makeUsageTotals({ calls: 30, promptTokens: 3_000_000 }) },
        { purpose: "subagent", ...makeUsageTotals() },
      ],
    })
    report.days[6] = { ...report.days[6], ...today }
    const get = mockUsage(report)
    renderSection()

    expect(await screen.findByText(/of 64k before summarizing/)).toBeVisible()
    expect(screen.getByText(/Context: 23.4k/)).toBeVisible()
    expect(screen.getByText("Model window: 1M")).toBeVisible()
    const meter = screen.getByRole("meter", { name: "Context used before summarizing" })
    expect(meter).toHaveAttribute("aria-valuenow", "23400")
    expect(meter).toHaveAttribute("aria-valuemax", "64000")
    expect(get).toHaveBeenCalledWith("/agents/{agentId}/usage", {
      params: { path: { agentId: "digest" }, query: { days: 7 } },
    })

    const table = screen.getByRole("table")
    const row = (name: string) =>
      within(table)
        .getAllByRole("row")
        .find((r) => within(r).queryByRole("rowheader", { name }))!
    expect(
      within(row("Input tokens"))
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["12.3k", "3.4M"])
    expect(
      within(row("From cache"))
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["50%", "25%"])
    expect(
      within(row("Output tokens"))
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["900", "45.6k"])
    expect(
      within(row("Model calls"))
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["4", "31"])

    // One bar per day, labelled with that day's numbers.
    expect(
      screen.getByRole("button", {
        name: "Today: 12.3k input, 50% from cache, 900 output, 4 calls",
      }),
    ).toBeInTheDocument()

    // Purposes in a fixed order with friendly names; unused ones left out.
    const purposes = screen.getByRole("list", { name: "Input tokens by purpose" })
    const items = within(purposes).getAllByRole("listitem")
    expect(items.map((li) => li.firstElementChild?.textContent)).toEqual([
      "Conversation",
      "Summaries",
    ])
    expect(items[0]).toHaveTextContent("3M 88%")
  })

  it("says when there's been nothing yet", async () => {
    mockUsage(makeUsageReport({ context: { tokens: 0, window: 262_144, compactAt: 64_000 } }))
    renderSection()

    expect(await screen.findByText("No requests yet")).toBeVisible()
    expect(screen.getByText("Model window: 262k")).toBeVisible()
    expect(screen.getByText("No model calls in the last 7 days.")).toBeVisible()
    expect(screen.queryByRole("meter")).not.toBeInTheDocument()
    expect(screen.queryByRole("table")).not.toBeInTheDocument()
  })
})

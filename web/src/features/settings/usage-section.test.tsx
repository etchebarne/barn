import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api, type Agent } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeUsageReport, makeUsageTotals } from "@/test/fixtures"

import { UsageSection } from "./usage-section"

const openAgentDetails = vi.hoisted(() => vi.fn<(agentId: string) => void>())
vi.mock("@/features/agents", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agents")>()),
  openAgentDetails,
}))

const digest = makeAgent({ id: "digest", name: "Weekly Digest" })
const research = makeAgent({ id: "research", name: "Research" })

type Report = ReturnType<typeof makeUsageReport>

function daysOf(init: unknown): number {
  if (init && typeof init === "object" && "params" in init) {
    const { params } = init
    if (params && typeof params === "object" && "query" in params) {
      const { query } = params
      if (query && typeof query === "object" && "days" in query) return Number(query.days)
    }
  }
  return 7
}

function mockUsage(byDays: Record<number, Report>) {
  return vi.spyOn(api, "GET").mockImplementation((...args: unknown[]) =>
    Promise.resolve({
      data: byDays[daysOf(args[1])],
      error: undefined,
      response: new Response(null),
    }),
  )
}

function renderSection(agents: Agent[] = [digest, research]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(queryKeys.agents, agents)
  render(
    <QueryClientProvider client={client}>
      <UsageSection />
    </QueryClientProvider>,
  )
}

const busy = makeUsageReport({
  total: makeUsageTotals({
    calls: 40,
    promptTokens: 1_250_000,
    cachedTokens: 500_000,
    completionTokens: 32_000,
  }),
  byAgent: [
    // Out of order on purpose: the table sorts by input.
    {
      agentId: "digest",
      ...makeUsageTotals({ calls: 10, promptTokens: 250_000, completionTokens: 2000 }),
    },
    {
      agentId: "research",
      ...makeUsageTotals({
        calls: 28,
        promptTokens: 990_000,
        cachedTokens: 495_000,
        completionTokens: 30_000,
      }),
    },
    { agentId: "gone", ...makeUsageTotals({ calls: 2, promptTokens: 10_000 }) },
  ],
  byPurpose: [{ purpose: "task", ...makeUsageTotals({ calls: 40, promptTokens: 1_250_000 }) }],
})

afterEach(() => {
  vi.restoreAllMocks()
  openAgentDetails.mockReset()
})

describe("usage settings", () => {
  it("shows totals and agents, most used first; a row opens that agent", async () => {
    mockUsage({ 7: busy })
    renderSection()

    const table = await screen.findByRole("table", { name: "Usage by agent" })
    const rows = within(table).getAllByRole("row").slice(1)
    expect(rows.map((r) => r.querySelector("td")?.textContent)).toEqual([
      "Research",
      "Weekly Digest",
      "?Deleted agent",
    ])
    expect(
      within(rows[0])
        .getAllByRole("cell")
        .map((c) => c.textContent),
    ).toEqual(["Research", "990k", "50%", "30k", "28"])
    // Totals: input, cached share, output, calls.
    for (const value of ["40%", "32k", "40"]) expect(screen.getByText(value)).toBeVisible()
    const purposes = screen.getByRole("list", { name: "Input tokens by purpose" })
    expect(purposes).toHaveTextContent("Tasks & app events1.3M 100%")

    await userEvent.click(within(rows[1]).getByRole("button", { name: "Weekly Digest" }))
    expect(openAgentDetails).toHaveBeenCalledWith("digest")
    // A deleted agent has nothing to open.
    expect(within(rows[2]).queryByRole("button")).not.toBeInTheDocument()
  })

  it("switches between 7 and 30 days", async () => {
    const get = mockUsage({
      7: makeUsageReport(),
      30: { ...busy, days: makeUsageReport({}, 30).days },
    })
    renderSection()

    expect(await screen.findByText(/No model calls in the last 7 days/)).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "30 days" }))
    expect(await screen.findByRole("table", { name: "Usage by agent" })).toBeVisible()
    expect(get).toHaveBeenCalledWith("/usage", { params: { query: { days: 30 } } })
    expect(screen.getAllByRole("button", { name: /input,/ })).toHaveLength(30)
  })
})

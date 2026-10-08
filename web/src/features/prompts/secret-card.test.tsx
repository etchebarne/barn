import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Message } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeMessage } from "@/test/fixtures"

import type { Prompt } from "./logic"
import { PromptCard } from "./prompt-card"

const VALUE = "sk-test-123"

function secretPrompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    kind: "secret",
    question: "I need your GitHub token.",
    options: [{ label: "Save" }, { label: "Not now" }],
    allowOther: false,
    status: "pending",
    answer: null,
    secret: {
      name: "GITHUB_TOKEN",
      description: "GitHub token with repo scope (github.com/settings/tokens)",
    },
    ...overrides,
  }
}

type Call = { path: string; body: unknown }

function renderCard(prompt: Prompt) {
  const message = makeMessage({
    id: "m1",
    chatId: "dm",
    author: { kind: "agent", agentId: "digest" },
    body: "",
    prompt,
  })
  const calls: Call[] = []
  vi.spyOn(api, "POST").mockImplementation((...args: unknown[]) => {
    const [path, init] = args
    const body = typeof init === "object" && init && "body" in init ? init.body : undefined
    calls.push({ path: String(path), body })
    const selected = String(path).endsWith("/secret") ? [0] : [1]
    const updated: Message = {
      ...message,
      prompt: { ...prompt, status: "answered", answer: { selected } },
    }
    return Promise.resolve({ data: updated, error: undefined, response: new Response(null) })
  })
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(queryKeys.agents, [makeAgent({ id: "digest", name: "Weekly Digest" })])
  client.setQueryData(queryKeys.messages("dm"), {
    pages: [{ messages: [message], hasMore: false }],
    pageParams: [undefined],
  })

  function Card() {
    // Reads the cached message like the message list does, so updates re-render it.
    const { data } = useQuery({
      queryKey: queryKeys.messages("dm"),
      queryFn: () => Promise.reject(new Error("not fetched in tests")),
      enabled: false,
      select: (d: { pages: { messages: Message[] }[] }) => d.pages[0]?.messages[0],
    })
    const current = data ?? message
    return (
      <TooltipProvider>
        {current.prompt && <PromptCard message={current} prompt={current.prompt} isLatest />}
      </TooltipProvider>
    )
  }
  const rootRoute = createRootRoute({ component: Card })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  const view = render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  return { calls, client, view }
}

afterEach(() => vi.restoreAllMocks())

describe("secret card", () => {
  it("shows what's asked for, with the description's address as a link", async () => {
    renderCard(secretPrompt())
    expect(await screen.findByText("Weekly Digest needs a secret")).toBeVisible()
    expect(screen.getByText("GITHUB_TOKEN")).toBeVisible()
    expect(screen.getByRole("link", { name: "github.com/settings/tokens" })).toHaveAttribute(
      "href",
      "https://github.com/settings/tokens",
    )
    const field = screen.getByLabelText("GITHUB_TOKEN")
    expect(field).toHaveAttribute("type", "password")
    expect(field).toHaveAttribute("autocomplete", "off")
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled()
    expect(screen.getByText(/never sees the value/)).toBeVisible()
  })

  it("posts the value to /secret on Enter, then shows only the name", async () => {
    const { calls, client, view } = renderCard(secretPrompt())
    await userEvent.type(await screen.findByLabelText("GITHUB_TOKEN"), `${VALUE}{Enter}`)
    expect(calls).toEqual([{ path: "/messages/{messageId}/secret", body: { value: VALUE } }])
    expect(await screen.findByText(/Saved as/)).toHaveTextContent("Saved as GITHUB_TOKEN")
    expect(screen.queryByLabelText("GITHUB_TOKEN")).not.toBeInTheDocument()
    expect(view.container.innerHTML).not.toContain(VALUE)
    // Nothing the app keeps holds the value.
    const cached = JSON.stringify([
      client
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data),
      client
        .getMutationCache()
        .getAll()
        .map((m) => m.state.variables),
    ])
    expect(cached).not.toContain(VALUE)
    expect(
      JSON.stringify(Object.keys(window.localStorage).map((k) => window.localStorage.getItem(k))),
    ).not.toContain(VALUE)
  })

  it("answers option 1 for Not now", async () => {
    const { calls } = renderCard(secretPrompt())
    await userEvent.click(await screen.findByRole("button", { name: "Not now" }))
    expect(calls).toEqual([{ path: "/messages/{messageId}/answer", body: { selected: [1] } }])
    expect(await screen.findByText("Not provided")).toBeVisible()
  })

  it("shows the answered and declined states without a field", async () => {
    renderCard(secretPrompt({ status: "answered", answer: { selected: [0] } }))
    expect(await screen.findByText(/Saved as/)).toBeVisible()
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument()
    expect(screen.queryByLabelText("GITHUB_TOKEN")).not.toBeInTheDocument()
  })

  it("shows the server's error inline and keeps the form", async () => {
    renderCard(secretPrompt())
    vi.spyOn(api, "POST").mockImplementation(() =>
      Promise.resolve({
        data: undefined,
        error: { message: "That doesn't look like a GitHub token" },
        response: new Response(null, { status: 400 }),
      }),
    )
    await userEvent.type(await screen.findByLabelText("GITHUB_TOKEN"), `${VALUE}{Enter}`)
    expect(await screen.findByRole("alert")).toHaveTextContent("doesn't look like")
    expect(screen.getByLabelText("GITHUB_TOKEN")).toBeVisible()
  })

  it("starts from a declined card too", async () => {
    renderCard(secretPrompt({ status: "answered", answer: { selected: [1] } }))
    expect(await screen.findByText("Not provided")).toBeVisible()
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument(),
    )
  })
})

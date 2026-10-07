import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import type { Prompt } from "./logic"
import { PromptCard } from "./prompt-card"

const approval: Prompt = {
  kind: "approval",
  question: "Archive Tracker?",
  options: [{ label: "Approve" }, { label: "Decline" }],
  allowOther: false,
  status: "pending",
  answer: null,
}

async function renderPrompt(prompt: Prompt) {
  const message = makeMessage({ body: prompt.question, prompt })
  const rootRoute = createRootRoute({
    component: () => <PromptCard message={message} prompt={prompt} isLatest />,
  })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={new QueryClient()}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await screen.findByRole("button", { name: /Decline/ })
}

describe("PromptCard approvals", () => {
  it("falls back to the plain question with Approve/Decline when there's no preview", async () => {
    await renderPrompt({ ...approval, preview: null })
    expect(screen.getByText("Archive Tracker?")).toBeVisible()
    expect(screen.getByRole("button", { name: "Approve" })).toBeVisible()
    expect(screen.queryByText("Needs approval")).not.toBeInTheDocument()
  })

  it("renders the structured preview when there is one", async () => {
    await renderPrompt({
      ...approval,
      preview: {
        appType: null,
        appName: null,
        title: "Archive agent",
        verb: "Archive",
        note: "Tracker stops working and its DM is removed.",
        fields: [{ key: "agent", label: "Agent", value: "Tracker" }],
        body: null,
      },
    })
    expect(screen.getByText("Archive agent")).toBeVisible()
    expect(screen.getByText("Needs approval")).toBeVisible()
    expect(screen.getByRole("button", { name: "Archive" })).toBeVisible()
    expect(screen.queryByText("Archive Tracker?")).not.toBeInTheDocument()
  })
})

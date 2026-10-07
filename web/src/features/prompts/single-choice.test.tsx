import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { makeMessage } from "@/test/fixtures"

import type { Prompt } from "./logic"
import { PromptCard } from "./prompt-card"

const single: Prompt = {
  kind: "single",
  question: "What should I call you?",
  options: [{ label: "Martin" }, { label: "Boss" }, { label: "Captain" }],
  allowOther: true,
  status: "pending",
  answer: null,
}

/** Captures answers sent to the server (the card's request), answering with the message. */
function captureAnswers(): unknown[] {
  const sent: unknown[] = []
  vi.spyOn(api, "POST").mockImplementation((...args: unknown[]) => {
    const [path, init] = args
    if (
      path === "/messages/{messageId}/answer" &&
      typeof init === "object" &&
      init &&
      "body" in init
    ) {
      sent.push(init.body)
    }
    return Promise.resolve({
      data: makeMessage({ prompt: { ...single, status: "answered" } }),
      error: undefined,
      response: new Response(null, { status: 200 }),
    })
  })
  return sent
}

async function renderSingle(prompt: Prompt = single) {
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
  await screen.findByRole("button", { name: "Submit" })
}

const option = (name: string) => screen.getByRole("button", { name: new RegExp(name) })

afterEach(() => vi.restoreAllMocks())

describe("single-choice prompts", () => {
  it("start with Submit disabled", async () => {
    await renderSingle()
    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled()
  })

  it("select on click or letter key without sending anything", async () => {
    const post = captureAnswers()
    await renderSingle()

    await userEvent.click(option("Boss"))
    expect(option("Boss")).toHaveAttribute("aria-pressed", "true")

    await userEvent.keyboard("c")
    expect(option("Captain")).toHaveAttribute("aria-pressed", "true")
    expect(option("Boss")).toHaveAttribute("aria-pressed", "false")

    expect(post).toEqual([])
    expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled()
  })

  it("clear the selection when the selected option is clicked again", async () => {
    await renderSingle()
    await userEvent.click(option("Martin"))
    await userEvent.click(option("Martin"))
    expect(option("Martin")).toHaveAttribute("aria-pressed", "false")
    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled()
  })

  it("send the selection with Submit", async () => {
    const post = captureAnswers()
    await renderSingle()
    await userEvent.click(option("Boss"))
    await userEvent.click(screen.getByRole("button", { name: "Submit" }))
    expect(post).toEqual([{ selected: [1] }])
  })

  it("submit with Enter, from the card or a focused option", async () => {
    const post = captureAnswers()
    await renderSingle()
    await userEvent.keyboard("a")
    const active = document.activeElement
    if (active instanceof HTMLElement) active.blur()
    await userEvent.keyboard("{Enter}")
    expect(post).toEqual([{ selected: [0] }])
  })

  it("submit with Enter on the focused option instead of unselecting it", async () => {
    const post = captureAnswers()
    await renderSingle()
    await userEvent.click(option("Captain"))
    await userEvent.keyboard("{Enter}")
    expect(post).toEqual([{ selected: [2] }])
  })

  it("let a typed answer replace the selection, and vice versa", async () => {
    const post = captureAnswers()
    await renderSingle()
    await userEvent.click(option("Boss"))
    await userEvent.click(screen.getByRole("button", { name: /Type your own/ }))
    await userEvent.type(screen.getByRole("textbox", { name: "Type your own…" }), "Marty")
    expect(option("Boss")).toHaveAttribute("aria-pressed", "false")

    await userEvent.click(option("Martin"))
    expect(screen.getByRole("textbox", { name: "Type your own…" })).toHaveValue("")

    await userEvent.type(screen.getByRole("textbox", { name: "Type your own…" }), "Marty{Enter}")
    expect(post).toEqual([{ text: "Marty" }])
  })

  it("leave multi-choice toggling unchanged", async () => {
    const post = captureAnswers()
    await renderSingle({ ...single, kind: "multi", allowOther: false })
    await userEvent.click(option("Martin"))
    await userEvent.click(option("Captain"))
    await userEvent.click(screen.getByRole("button", { name: "Submit" }))
    expect(post).toEqual([{ selected: [0, 2] }])
  })
})

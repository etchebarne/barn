import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/lib/api-client"
import { makeAgent } from "@/test/fixtures"

import type { AgentSecret } from "./api"
import { SecretsSection, secretNameError, toSecretName } from "./secrets-section"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

const agent = makeAgent({ id: "digest", name: "Weekly Digest" })
const github: AgentSecret = {
  name: "GITHUB_TOKEN",
  description: "Repo scope",
  createdAt: "2026-10-01T10:00:00Z",
  updatedAt: "2026-10-01T10:00:00Z",
}

type Call = { method: string; path: string; init: unknown }

function mockApi(secrets: AgentSecret[]) {
  const calls: Call[] = []
  vi.spyOn(api, "GET").mockImplementation(() =>
    Promise.resolve({ data: secrets, error: undefined, response: new Response(null) }),
  )
  const write =
    (method: string) =>
    (...args: unknown[]) => {
      const [path, init] = args
      calls.push({ method, path: String(path), init })
      return Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(null, { status: 204 }),
      })
    }
  vi.spyOn(api, "PUT").mockImplementation(write("PUT"))
  vi.spyOn(api, "DELETE").mockImplementation(write("DELETE"))
  return calls
}

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <SecretsSection agent={agent} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("secret names", () => {
  it("upper-case as you type, with spaces and dashes as underscores", () => {
    expect(toSecretName("github token")).toBe("GITHUB_TOKEN")
    expect(toSecretName("open-ai_key2")).toBe("OPEN_AI_KEY2")
  })

  it("explain what's wrong", () => {
    expect(secretNameError("")).toBe("Enter a name.")
    expect(secretNameError("1KEY")).toBe("Start with a letter.")
    expect(secretNameError("API.KEY")).toBe("Use capital letters, digits and _ only.")
    expect(secretNameError("A")).toBe("Use 2 to 64 characters.")
    expect(secretNameError("GITHUB_TOKEN")).toBeNull()
  })
})

describe("secrets section", () => {
  it("lists names and descriptions, never values, or explains there are none", async () => {
    mockApi([github])
    renderSection()
    const list = await screen.findByRole("list", { name: "Secrets" })
    expect(within(list).getByText("GITHUB_TOKEN")).toBeVisible()
    expect(within(list).getByText("Repo scope")).toBeVisible()
    expect(within(list).getByText(/^Updated /)).toBeVisible()
    expect(screen.getByText(/as environment variables in its computer/)).toBeVisible()
  })

  it("shows an empty state", async () => {
    mockApi([])
    renderSection()
    expect(await screen.findByText(/No secrets yet/)).toBeVisible()
  })

  it("adds a secret with a PUT to its name", async () => {
    const calls = mockApi([])
    renderSection()
    await userEvent.click(await screen.findByRole("button", { name: "Add secret" }))
    const form = screen.getByRole("form", { name: "Add secret" })
    await userEvent.type(within(form).getByLabelText("Name"), "openai key")
    expect(within(form).getByLabelText("Name")).toHaveValue("OPENAI_KEY")
    await userEvent.type(within(form).getByLabelText("Description (optional)"), "For embeddings")
    await userEvent.type(within(form).getByLabelText("Value"), "sk-test-123")
    await userEvent.click(within(form).getByRole("button", { name: "Save" }))
    expect(calls).toEqual([
      {
        method: "PUT",
        path: "/agents/{agentId}/secrets/{name}",
        init: {
          params: { path: { agentId: "digest", name: "OPENAI_KEY" } },
          body: { value: "sk-test-123", description: "For embeddings" },
        },
      },
    ])
  })

  it("won't save a bad name", async () => {
    const calls = mockApi([])
    renderSection()
    await userEvent.click(await screen.findByRole("button", { name: "Add secret" }))
    const form = screen.getByRole("form", { name: "Add secret" })
    await userEvent.type(within(form).getByLabelText("Name"), "9lives")
    await userEvent.type(within(form).getByLabelText("Value"), "x")
    expect(within(form).getByText("Start with a letter.")).toBeVisible()
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled()
    expect(calls).toEqual([])
  })

  it("replaces a value inline, keeping the description", async () => {
    const calls = mockApi([github])
    renderSection()
    await userEvent.click(await screen.findByRole("button", { name: "Replace GITHUB_TOKEN" }))
    await userEvent.type(screen.getByLabelText("New value for GITHUB_TOKEN"), "ghp_new{Enter}")
    expect(calls).toEqual([
      {
        method: "PUT",
        path: "/agents/{agentId}/secrets/{name}",
        init: {
          params: { path: { agentId: "digest", name: "GITHUB_TOKEN" } },
          body: { value: "ghp_new" },
        },
      },
    ])
  })

  it("deletes after an inline confirm", async () => {
    const calls = mockApi([github])
    renderSection()
    await userEvent.click(await screen.findByRole("button", { name: "Delete GITHUB_TOKEN" }))
    expect(calls).toEqual([])
    const confirm = screen.getByRole("alertdialog", { name: "Delete GITHUB_TOKEN?" })
    await userEvent.click(within(confirm).getByRole("button", { name: "Delete" }))
    expect(calls).toEqual([
      {
        method: "DELETE",
        path: "/agents/{agentId}/secrets/{name}",
        init: { params: { path: { agentId: "digest", name: "GITHUB_TOKEN" } } },
      },
    ])
  })
})

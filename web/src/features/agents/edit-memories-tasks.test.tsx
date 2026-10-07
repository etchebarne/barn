import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { api } from "@/lib/api-client"
import { makeAgent } from "@/test/fixtures"

import type { Memory } from "./api"
import { MemoriesSection } from "./memories-section"
import type { Task } from "./schedule"
import { TasksSection } from "./tasks-section"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

const agent = makeAgent({ id: "digest", name: "Weekly Digest" })

const memory: Memory = {
  id: "m1",
  text: "Martin likes numbers first.",
  createdAt: "2026-10-01T10:00:00Z",
}

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "t1",
    agentId: "digest",
    name: "Weekday check-in",
    purpose: "Give Martin a quick catch-up",
    kind: "cron",
    cron: "0 9 * * 1-5",
    at: null,
    enabled: true,
    signal: null,
    nextFireAt: "2026-10-08T09:00:00Z",
    lastFiredAt: null,
    ...overrides,
  }
}

type Call = { method: string; path: string; body: unknown }
type Reply = { status: number; data?: Memory | Task; error?: { message: string } }

function textOf(body: unknown): string {
  return typeof body === "object" && body && "text" in body && typeof body.text === "string"
    ? body.text
    : ""
}

/** Answers GETs with the seed lists and records writes, answering with `reply`. */
function mockApi({
  memories = [memory],
  tasks = [task()],
  reply = (call: Call): Reply => ({
    status: call.method === "POST" ? 201 : 200,
    data: Object.assign(task({ id: "new" }), call.body),
  }),
}: {
  memories?: Memory[]
  tasks?: Task[]
  reply?: (call: Call) => Reply
} = {}) {
  const calls: Call[] = []
  vi.spyOn(api, "GET").mockImplementation((...args: unknown[]) => {
    const data = String(args[0]).includes("memories") ? memories : tasks
    return Promise.resolve({ data, error: undefined, response: new Response(null) })
  })
  const write =
    (method: string) =>
    (...args: unknown[]) => {
      const [path, init] = args
      const body = typeof init === "object" && init && "body" in init ? init.body : undefined
      const call = { method, path: String(path), body }
      calls.push(call)
      const { status, data, error } = reply(call)
      const response = new Response(null, { status })
      return Promise.resolve(
        error ? { data: undefined, error, response } : { data, error: undefined, response },
      )
    }
  vi.spyOn(api, "POST").mockImplementation(write("POST"))
  // PATCH's overloads type the mock as the first path's (sidebar categories); answers here are
  // memories and tasks.
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- see above
  vi.spyOn(api, "PATCH").mockImplementation(write("PATCH") as never)
  return calls
}

function renderSection(section: "memories" | "tasks") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        {section === "memories" ? (
          <MemoriesSection agent={agent} />
        ) : (
          <TasksSection agent={agent} />
        )}
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("memories", () => {
  it("adds a memory with Enter", async () => {
    const calls = mockApi({
      reply: (call) => ({
        status: 201,
        data: { id: "m2", text: textOf(call.body), createdAt: memory.createdAt },
      }),
    })
    renderSection("memories")
    await userEvent.click(await screen.findByRole("button", { name: "Add memory" }))
    await userEvent.type(
      screen.getByRole("textbox", { name: "New memory" }),
      "  Reports go out on Fridays.{Enter}",
    )

    expect(calls).toEqual([
      {
        method: "POST",
        path: "/agents/{agentId}/memories",
        body: { text: "Reports go out on Fridays." },
      },
    ])
    expect(await screen.findByText("Reports go out on Fridays.")).toBeVisible()
    expect(screen.queryByRole("textbox", { name: "New memory" })).not.toBeInTheDocument()
  })

  it("edits a memory and sends the new text; Escape cancels", async () => {
    const calls = mockApi({
      reply: (call) => ({
        status: 200,
        data: { ...memory, text: textOf(call.body) },
      }),
    })
    renderSection("memories")
    await userEvent.click(await screen.findByRole("button", { name: "Edit memory" }))
    await userEvent.keyboard("{Escape}")
    expect(screen.queryByRole("textbox", { name: "Memory" })).not.toBeInTheDocument()

    await userEvent.click(screen.getByText("Martin likes numbers first."))
    const box = screen.getByRole("textbox", { name: "Memory" })
    await userEvent.clear(box)
    await userEvent.type(box, "Martin likes the totals first.")
    await userEvent.click(screen.getByRole("button", { name: "Save" }))

    expect(calls).toEqual([
      {
        method: "PATCH",
        path: "/agents/{agentId}/memories/{memoryId}",
        body: { text: "Martin likes the totals first." },
      },
    ])
    expect(await screen.findByText("Martin likes the totals first.")).toBeVisible()
  })

  it("shows the server's error inline", async () => {
    mockApi({ reply: () => ({ status: 400, error: { message: "That memory is too long" } }) })
    renderSection("memories")
    await userEvent.click(await screen.findByRole("button", { name: "Add memory" }))
    await userEvent.type(screen.getByRole("textbox", { name: "New memory" }), "x{Enter}")
    expect(await screen.findByText("That memory is too long")).toBeVisible()
  })
})

describe("tasks", () => {
  it("creates a task from a preset and sends its cron", async () => {
    const calls = mockApi({ tasks: [] })
    renderSection("tasks")
    await userEvent.click(await screen.findByRole("button", { name: "New task" }))
    const form = screen.getByRole("form", { name: "New task" })
    await userEvent.type(within(form).getByLabelText("Name"), "Morning brief")
    await userEvent.type(within(form).getByLabelText("What it does"), "Summarize the inbox")
    await userEvent.selectOptions(within(form).getByLabelText("Repeats"), "Weekdays")
    fireEvent.change(within(form).getByLabelText("Time"), { target: { value: "08:30" } })
    expect(within(form).getByText("At 08:30 AM, Monday through Friday")).toBeVisible()
    await userEvent.click(within(form).getByRole("button", { name: "Create task" }))

    expect(calls).toEqual([
      {
        method: "POST",
        path: "/agents/{agentId}/tasks",
        body: { name: "Morning brief", purpose: "Summarize the inbox", cron: "30 8 * * 1-5" },
      },
    ])
    await waitFor(() => expect(screen.queryByRole("form")).not.toBeInTheDocument())
    expect(screen.getByText("Morning brief")).toBeVisible()
  })

  it("builds weekly and hourly crons", async () => {
    const calls = mockApi({ tasks: [] })
    renderSection("tasks")
    await userEvent.click(await screen.findByRole("button", { name: "New task" }))
    const form = screen.getByRole("form", { name: "New task" })
    await userEvent.type(within(form).getByLabelText("Name"), "Hourly")
    await userEvent.type(within(form).getByLabelText("What it does"), "Check")
    await userEvent.selectOptions(within(form).getByLabelText("Repeats"), "Every hour")
    fireEvent.change(within(form).getByLabelText("at minute"), { target: { value: "15" } })
    await userEvent.click(within(form).getByRole("button", { name: "Create task" }))
    expect(calls[0]?.body).toMatchObject({ cron: "15 * * * *" })
  })

  it("switches a task to Once and sends `at` instead of cron", async () => {
    const calls = mockApi()
    renderSection("tasks")
    await userEvent.click(await screen.findByRole("button", { name: "Edit task Weekday check-in" }))
    const form = screen.getByRole("form", { name: "Edit Weekday check-in" })
    // The preset is recognized from the cron.
    expect(within(form).getByLabelText("Repeats")).toHaveValue("weekdays")
    expect(within(form).getByLabelText("Time")).toHaveValue("09:00")

    await userEvent.click(within(form).getByRole("button", { name: "Once" }))
    fireEvent.change(within(form).getByLabelText("Date"), { target: { value: "2026-10-20" } })
    fireEvent.change(within(form).getByLabelText("Time"), { target: { value: "15:30" } })
    await userEvent.click(within(form).getByRole("button", { name: "Save" }))

    expect(calls).toEqual([
      {
        method: "PATCH",
        path: "/tasks/{taskId}",
        body: {
          name: "Weekday check-in",
          purpose: "Give Martin a quick catch-up",
          at: "2026-10-20 15:30",
        },
      },
    ])
  })

  it("shows a signal task's trigger read-only and never sends a schedule", async () => {
    const signalTask = task({
      kind: "signal",
      cron: null,
      signal: "slack.app_mention in Slack (work)",
      nextFireAt: null,
    })
    const calls = mockApi({ tasks: [signalTask] })
    renderSection("tasks")
    await userEvent.click(await screen.findByRole("button", { name: "Edit task Weekday check-in" }))
    const form = screen.getByRole("form", { name: "Edit Weekday check-in" })
    expect(within(form).getByText(/When slack\.app_mention in Slack \(work\)/)).toBeVisible()
    expect(within(form).queryByRole("button", { name: "Once" })).not.toBeInTheDocument()

    const name = within(form).getByLabelText("Name")
    await userEvent.clear(name)
    await userEvent.type(name, "Answer mentions")
    await userEvent.click(within(form).getByRole("button", { name: "Save" }))
    expect(calls).toEqual([
      {
        method: "PATCH",
        path: "/tasks/{taskId}",
        body: { name: "Answer mentions", purpose: "Give Martin a quick catch-up" },
      },
    ])
  })

  it("shows the server's validation error inline and keeps the form", async () => {
    mockApi({ reply: () => ({ status: 400, error: { message: "That time is in the past" } }) })
    renderSection("tasks")
    await userEvent.click(await screen.findByRole("button", { name: "Edit task Weekday check-in" }))
    const form = screen.getByRole("form", { name: "Edit Weekday check-in" })
    await userEvent.click(within(form).getByRole("button", { name: "Once" }))
    await userEvent.click(within(form).getByRole("button", { name: "Save" }))
    expect(await within(form).findByText("That time is in the past")).toBeVisible()
  })
})

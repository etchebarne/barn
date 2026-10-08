import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import type { TaskRun } from "./api"
import { RunRow } from "./run-row"

const now = new Date("2026-10-08T12:00:00Z")

function run(overrides: Partial<TaskRun> = {}): TaskRun {
  return {
    id: "r1",
    taskId: "t1",
    agentId: "a1",
    trigger: "schedule",
    outcome: "acted",
    detail: "",
    startedAt: "2026-10-08T11:00:00Z",
    finishedAt: "2026-10-08T11:00:42Z",
    ...overrides,
  }
}

function renderRow(r: TaskRun) {
  return render(
    <TooltipProvider>
      <ul>
        <RunRow run={r} now={now} />
      </ul>
    </TooltipProvider>,
  )
}

describe("RunRow", () => {
  it("says how a run went, when, and how long it took", () => {
    renderRow(run({ trigger: "manual" }))
    expect(screen.getByText("Did something")).toBeVisible()
    expect(screen.getByText("1 hour ago")).toBeVisible()
    expect(screen.getByText("took 42s")).toBeVisible()
    expect(screen.getByText("Run by you")).toBeVisible()
  })

  it("shows why a run failed, expanding on click", async () => {
    renderRow(run({ outcome: "failed", detail: "the model provider is down" }))
    expect(screen.getByText("Failed")).toBeVisible()
    const detail = screen.getByRole("button", { name: "the model provider is down" })
    expect(detail).toHaveAttribute("aria-expanded", "false")
    await userEvent.click(detail)
    expect(detail).toHaveAttribute("aria-expanded", "true")
  })

  it("has no duration while running", () => {
    renderRow(run({ outcome: "running", finishedAt: null }))
    expect(screen.getByText("Running")).toBeVisible()
    expect(screen.queryByText(/^took/)).toBeNull()
  })
})

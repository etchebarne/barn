import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"

import type { Task } from "./schedule"
import { TaskList } from "./tasks-section"

const now = new Date("2026-10-07T12:00:00Z")

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "t1",
    agentId: "a1",
    name: "Weekday check-in",
    purpose: "Give Martin a quick catch-up",
    kind: "cron",
    cron: "1 10 * * 1-5",
    at: null,
    enabled: true,
    signal: null,
    nextFireAt: "2026-10-07T15:00:00Z",
    lastFiredAt: null,
    ...overrides,
  }
}

function renderList(tasks: Task[], handlers = {}) {
  return render(
    <TooltipProvider>
      <TaskList
        agentName="openbot"
        tasks={tasks}
        now={now}
        onToggle={vi.fn<(task: Task, enabled: boolean) => void>()}
        onDelete={vi.fn<(task: Task) => void>()}
        {...handlers}
      />
    </TooltipProvider>,
  )
}

describe("TaskList", () => {
  it("explains how to create tasks when there are none", () => {
    renderList([])
    expect(
      screen.getByText(/No tasks yet\. Create one, or ask openbot to do something on a schedule/),
    ).toBeVisible()
  })

  it("shows the schedule and next run of an active task", () => {
    renderList([task()])
    expect(screen.getByText("Weekday check-in")).toBeVisible()
    expect(screen.getByText("At 10:01 AM, Monday through Friday")).toBeVisible()
    expect(screen.getByText(/^Next: in \d+ hours?$/)).toBeVisible()
    expect(screen.getByRole("switch", { name: "Pause task Weekday check-in" })).toBeChecked()
  })

  it("shows Paused and an unchecked switch for a disabled task", () => {
    renderList([task({ enabled: false })])
    expect(screen.getByText("Paused")).toBeVisible()
    expect(screen.queryByText(/^Next:/)).not.toBeInTheDocument()
    expect(screen.getByRole("switch", { name: "Resume task Weekday check-in" })).not.toBeChecked()
  })

  it("shows what a task's check watches", () => {
    renderList([task({ check: "curl -s https://status.example.com | jq .state" })])
    expect(screen.getByText("Only wakes when this changes:")).toBeVisible()
    expect(screen.getByText("curl -s https://status.example.com | jq .state")).toBeVisible()
  })

  it("shows no check line for a task without one", () => {
    renderList([task()])
    expect(screen.queryByText(/Only wakes when this changes/)).not.toBeInTheDocument()
  })

  it("toggles and deletes (after confirming)", async () => {
    const onToggle = vi.fn<(task: Task, enabled: boolean) => void>()
    const onDelete = vi.fn<(task: Task) => void>()
    renderList([task()], { onToggle, onDelete })

    await userEvent.click(screen.getByRole("switch", { name: "Pause task Weekday check-in" }))
    expect(onToggle).toHaveBeenCalledWith(expect.objectContaining({ id: "t1" }), false)

    await userEvent.click(screen.getByRole("button", { name: "Delete task Weekday check-in" }))
    expect(onDelete).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole("button", { name: "Delete" }))
    expect(onDelete).toHaveBeenCalledWith(expect.objectContaining({ id: "t1" }))
  })
})

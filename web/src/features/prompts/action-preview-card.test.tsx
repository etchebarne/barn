import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { ActionPreviewCard } from "./action-preview-card"
import type { Prompt, PromptAnswer } from "./logic"
import type { ActionPreview } from "./preview"

const preview: ActionPreview = {
  appType: "slack",
  appName: "Work Slack",
  title: "Slack message",
  verb: "Send message",
  note: "Posts as the openbot app.",
  fields: [
    { key: "channel", label: "To", value: "#bot-ception" },
    { key: "thread_ts", label: "Thread", value: "Reply in 'deploy is green'" },
    { key: "unfurl", label: "Unfurl links", value: false },
  ],
  body: { key: "text", label: "Message", value: "Deploy finished.\nAll checks passed." },
}

function prompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    kind: "approval",
    question: "Send a Slack message?",
    options: [{ label: "Approve" }, { label: "Decline" }],
    allowOther: false,
    status: "pending",
    answer: null,
    preview,
    ...overrides,
  }
}

function renderCard(overrides: Partial<Parameters<typeof ActionPreviewCard>[0]> = {}) {
  const onAnswer = vi.fn<(answer: PromptAnswer) => void>()
  render(
    <ActionPreviewCard
      prompt={prompt()}
      preview={preview}
      isLatest
      disabled={false}
      onAnswer={onAnswer}
      onDismiss={vi.fn<() => void>()}
      {...overrides}
    />,
  )
  return { onAnswer }
}

describe("ActionPreviewCard", () => {
  it("shows the action, its fields in order, the body and the note", () => {
    renderCard()
    expect(screen.getByText("Slack message")).toBeVisible()
    expect(screen.getByText("· Work Slack")).toBeVisible()
    expect(screen.getByText("Needs approval")).toBeVisible()

    const labels = screen.getAllByRole("term").map((el) => el.textContent)
    expect(labels).toEqual(["To", "Thread", "Unfurl links"])
    expect(screen.getByText("#bot-ception")).toBeVisible()
    expect(screen.getByText("No")).toBeVisible()

    const body = screen.getByRole("group", { name: "Message" })
    expect(within(body).getByText(/Deploy finished\.\s+All checks passed\./)).toBeVisible()
    expect(screen.getByText("Posts as the openbot app.")).toBeVisible()
  })

  it("approves with the verb button and declines with Decline", async () => {
    const { onAnswer } = renderCard()
    await userEvent.click(screen.getByRole("button", { name: "Send message" }))
    expect(onAnswer).toHaveBeenLastCalledWith({ selected: [0] })
    await userEvent.click(screen.getByRole("button", { name: "Decline" }))
    expect(onAnswer).toHaveBeenLastCalledWith({ selected: [1] })
  })

  it("disables the buttons while answering", () => {
    renderCard({ disabled: true })
    expect(screen.getByRole("button", { name: "Send message" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Decline" })).toBeDisabled()
  })

  const settled: [Partial<Prompt>, string][] = [
    [{ status: "answered", answer: { selected: [0] } }, "Approved"],
    [{ status: "answered", answer: { selected: [1] } }, "Declined"],
    [{ status: "dismissed" }, "Dismissed"],
  ]

  it.each(settled)("shows the %o state without buttons", (state, label) => {
    renderCard({ prompt: prompt(state) })
    expect(screen.getByText(label)).toBeVisible()
    expect(screen.queryByRole("button", { name: "Send message" })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Dismiss request" })).not.toBeInTheDocument()
    // Still readable.
    expect(screen.getByText("#bot-ception")).toBeVisible()
  })

  it("uses openbot's icon and no app name for openbot's own actions", () => {
    renderCard({
      preview: {
        ...preview,
        appType: "openbot",
        appName: null,
        title: "Delete agent",
        verb: "Delete",
        body: null,
      },
    })
    expect(screen.getByText("Delete agent")).toBeVisible()
    expect(screen.queryByText(/· /)).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Delete" })).toBeVisible()
  })

  it("collapses very long values behind Show more", async () => {
    const long = Array.from({ length: 20 }, (_, i) => `line ${i}`).join("\n")
    renderCard({ preview: { ...preview, body: { key: "text", label: "Message", value: long } } })
    const toggle = screen.getByRole("button", { name: "Show more" })
    await userEvent.click(toggle)
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute(
      "aria-expanded",
      "true",
    )
  })

  it("offers a quieter Always allow when there's a third option", async () => {
    const threeOptions = prompt({
      options: [{ label: "Approve" }, { label: "Decline" }, { label: "Always allow" }],
    })
    const { onAnswer } = renderCard({ prompt: threeOptions })
    const always = screen.getByRole("button", { name: "Always allow" })
    expect(screen.getByRole("button", { name: "Send message" })).toBeVisible()
    await userEvent.click(always)
    expect(onAnswer).toHaveBeenCalledWith({ selected: [2] })
  })

  it("has no Always allow with only two options", () => {
    renderCard()
    expect(screen.queryByRole("button", { name: "Always allow" })).not.toBeInTheDocument()
  })

  it("shows Always allowed once answered with the third option", () => {
    renderCard({
      prompt: prompt({
        options: [{ label: "Approve" }, { label: "Decline" }, { label: "Always allow" }],
        status: "answered",
        answer: { selected: [2] },
      }),
    })
    expect(screen.getByText("Always allowed")).toBeVisible()
    expect(screen.queryByRole("button", { name: "Always allow" })).not.toBeInTheDocument()
  })

  it("renders an agent's standing-approval proposal as a normal two-option card", async () => {
    const proposal: ActionPreview = {
      appType: null,
      appName: null,
      title: "Always allow",
      verb: "Always allow",
      note: "openbot won't ask before doing this again. Remove it any time in openbot's settings.",
      fields: [
        { key: "action", label: "Action", value: "Slack message · Work Slack" },
        { key: "only_when", label: "Only when", value: "Channel is #bot-ception" },
      ],
      body: null,
    }
    const { onAnswer } = renderCard({ prompt: prompt({ preview: proposal }), preview: proposal })
    expect(screen.getAllByRole("term").map((t) => t.textContent)).toEqual(["Action", "Only when"])
    expect(screen.getByText("Channel is #bot-ception")).toBeVisible()
    const buttons = screen.getAllByRole("button", { name: "Always allow" })
    expect(buttons).toHaveLength(1)
    await userEvent.click(buttons[0])
    expect(onAnswer).toHaveBeenCalledWith({ selected: [0] })
  })
})

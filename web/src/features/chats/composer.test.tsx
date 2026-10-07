import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { describe, expect, it, vi } from "vitest"

import { Composer } from "./composer"

const members = [
  { id: "a", name: "Alpha" },
  { id: "b", name: "Beta" },
  { id: "c", name: "barn" },
]

function Harness({
  onSend,
  mentionCandidates,
}: {
  onSend: (text: string) => void
  mentionCandidates?: typeof members
}) {
  const [value, setValue] = useState("")
  return (
    <Composer
      value={value}
      onChange={setValue}
      onSend={onSend}
      mentionCandidates={mentionCandidates}
    />
  )
}

describe("Composer", () => {
  it("sends the trimmed text on Enter and clears the field", async () => {
    const onSend = vi.fn<(text: string) => void>()
    render(<Harness onSend={onSend} />)
    const box = screen.getByRole("textbox", { name: "Message" })

    await userEvent.type(box, "  hello there  {Enter}")

    expect(onSend).toHaveBeenCalledWith("hello there")
    expect(box).toHaveValue("")
  })

  it("inserts a newline on Shift+Enter instead of sending", async () => {
    const onSend = vi.fn<(text: string) => void>()
    render(<Harness onSend={onSend} />)
    const box = screen.getByRole("textbox", { name: "Message" })

    await userEvent.type(box, "line one{Shift>}{Enter}{/Shift}line two")

    expect(onSend).not.toHaveBeenCalled()
    expect(box).toHaveValue("line one\nline two")
  })

  it("doesn't send whitespace-only messages", async () => {
    const onSend = vi.fn<(text: string) => void>()
    render(<Harness onSend={onSend} />)

    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "   {Enter}")
    await userEvent.click(screen.getByRole("button", { name: "Send message" }))

    expect(onSend).not.toHaveBeenCalled()
  })

  it("sends with the send button", async () => {
    const onSend = vi.fn<(text: string) => void>()
    render(<Harness onSend={onSend} />)

    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "hi")
    await userEvent.click(screen.getByRole("button", { name: "Send message" }))

    expect(onSend).toHaveBeenCalledWith("hi")
  })

  describe("@mentions", () => {
    it("suggests group members filtered by what's typed", async () => {
      render(<Harness onSend={vi.fn<(text: string) => void>()} mentionCandidates={members} />)
      const box = screen.getByRole("combobox", { name: "Message" })

      await userEvent.type(box, "hi @")
      expect(await screen.findAllByRole("option")).toHaveLength(3)

      await userEvent.type(box, "b")
      const options = screen.getAllByRole("option").map((o) => o.textContent)
      expect(options).toEqual(["Beta", "barn"])
    })

    it("inserts the mention on Enter instead of sending, then Enter sends", async () => {
      const onSend = vi.fn<(text: string) => void>()
      render(<Harness onSend={onSend} mentionCandidates={members} />)
      const box = screen.getByRole("combobox", { name: "Message" })

      await userEvent.type(box, "hey @al")
      await screen.findByRole("option", { name: /Alpha/ })
      await userEvent.keyboard("{Enter}")

      expect(onSend).not.toHaveBeenCalled()
      expect(box).toHaveValue("hey @Alpha ")
      expect(screen.queryByRole("option")).not.toBeInTheDocument()

      await userEvent.keyboard("ship it{Enter}")
      expect(onSend).toHaveBeenCalledWith("hey @Alpha ship it")
    })

    it("moves with the arrow keys and inserts with Tab", async () => {
      render(<Harness onSend={vi.fn<(text: string) => void>()} mentionCandidates={members} />)
      const box = screen.getByRole("combobox", { name: "Message" })

      await userEvent.type(box, "@")
      await screen.findAllByRole("option")
      await userEvent.keyboard("{ArrowDown}{Tab}")

      expect(box).toHaveValue("@Beta ")
    })

    it("closes on Escape so Enter sends again", async () => {
      const onSend = vi.fn<(text: string) => void>()
      render(<Harness onSend={onSend} mentionCandidates={members} />)
      const box = screen.getByRole("combobox", { name: "Message" })

      await userEvent.type(box, "mail @b")
      await screen.findAllByRole("option")
      await userEvent.keyboard("{Escape}")
      expect(screen.queryByRole("option")).not.toBeInTheDocument()

      await userEvent.keyboard("{Enter}")
      expect(onSend).toHaveBeenCalledWith("mail @b")
    })

    it("offers no suggestions in DMs", async () => {
      render(<Harness onSend={vi.fn<(text: string) => void>()} />)
      await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "@")
      expect(screen.queryByRole("option")).not.toBeInTheDocument()
    })
  })
})

import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { describe, expect, it, vi } from "vitest"

import { Composer } from "./composer"

function Harness({ onSend }: { onSend: (text: string) => void }) {
  const [value, setValue] = useState("")
  return <Composer value={value} onChange={setValue} onSend={onSend} />
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
})

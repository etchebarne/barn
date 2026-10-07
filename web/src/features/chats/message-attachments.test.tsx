import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import type { Attachment } from "@/lib/attachments"
import { makeAgent, makeChat, makeMessage } from "@/test/fixtures"

import { MessageAttachments, singleImageSize } from "./message-attachments"
import { MessageRow } from "./message-row"
import { chatPreview } from "./preview"

const img = (id: string, width: number | null = 1600, height: number | null = 900): Attachment => ({
  id,
  name: `${id}.png`,
  mime: "image/png",
  size: 2048,
  url: `/api/attachments/${id}`,
  width,
  height,
})
const pdf: Attachment = {
  id: "p1",
  name: "report.pdf",
  mime: "application/pdf",
  size: 3.2 * 1024 * 1024,
  url: "/api/attachments/p1",
  width: null,
  height: null,
}
const zip: Attachment = { ...pdf, id: "z1", name: "logs.zip", mime: "application/zip", size: 512 }

function wrap(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>{ui}</TooltipProvider>
    </QueryClientProvider>,
  )
}

describe("MessageAttachments", () => {
  it("shows one image larger, keeping its aspect ratio", () => {
    expect(singleImageSize(img("a", 1600, 900))).toEqual({ width: 320, height: 180 })
    expect(singleImageSize(img("a", 200, 100))).toEqual({ width: 200, height: 100 })
    expect(singleImageSize(img("a", null, null))).toBeNull()
    wrap(<MessageAttachments attachments={[img("a")]} align="start" />)
    expect(screen.getByRole("button", { name: "Open a.png" })).toHaveStyle({ width: "320px" })
  })

  it("puts several images in a grid, images before files", () => {
    wrap(<MessageAttachments attachments={[zip, img("a"), img("b"), img("c")]} align="start" />)
    const grid = screen.getByRole("list", { name: "Images" })
    expect(grid).toHaveClass("grid-cols-3")
    expect(
      within(grid)
        .getAllByRole("img")
        .map((i) => i.getAttribute("alt")),
    ).toEqual(["a.png", "b.png", "c.png"])
    const lists = screen.getAllByRole("list").map((l) => l.getAttribute("aria-label"))
    expect(lists).toEqual(["Images", "Files"])
  })

  it("shows files as cards with size and download, PDFs opening in a new tab", () => {
    wrap(<MessageAttachments attachments={[pdf, zip]} align="start" />)
    expect(screen.getByText("3.2 MB")).toBeVisible()
    expect(screen.getByRole("link", { name: "report.pdf" })).toHaveAttribute("target", "_blank")
    expect(screen.getByRole("link", { name: "Download report.pdf" })).toHaveAttribute(
      "href",
      "/api/attachments/p1?download=1",
    )
    expect(screen.queryByRole("link", { name: "logs.zip" })).not.toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Download logs.zip" })).toBeVisible()
  })

  it("opens an image in a lightbox, with download, and closes it on Esc", async () => {
    wrap(<MessageAttachments attachments={[img("a"), img("b")]} align="end" />)
    await userEvent.click(screen.getByRole("button", { name: "Open b.png" }))
    const dialog = await screen.findByRole("dialog")
    expect(within(dialog).getByRole("img", { name: "b.png" })).toHaveAttribute(
      "src",
      "/api/attachments/b",
    )
    expect(within(dialog).getByRole("link", { name: /Download/ })).toHaveAttribute(
      "href",
      "/api/attachments/b?download=1",
    )
    await userEvent.keyboard("{Escape}")
    await expect.poll(() => screen.queryByRole("dialog")).toBeNull()
  })
})

describe("messages with attachments", () => {
  it("show only the attachments when the body is empty", () => {
    const message = makeMessage({
      author: { kind: "user", agentId: null },
      body: "",
      attachments: [img("a"), pdf],
    })
    const { container } = wrap(
      <MessageRow
        row={{ message, startsRun: true, endsRun: true, startsDay: false }}
        agents={new Map()}
        showAuthorName={false}
        isLatest={false}
      />,
    )
    expect(screen.getByRole("button", { name: "Open a.png" })).toBeVisible()
    expect(screen.getByText("report.pdf")).toBeVisible()
    expect(container.querySelector("[data-slot=bubble-content]")).toBeNull()
  })

  it("show attachments above the text for agents too", () => {
    const message = makeMessage({ body: "Here's the chart", attachments: [img("a")] })
    wrap(
      <MessageRow
        row={{ message, startsRun: true, endsRun: true, startsDay: false }}
        agents={new Map([["agent-1", makeAgent()]])}
        showAuthorName={false}
        isLatest={false}
      />,
    )
    expect(screen.getByRole("button", { name: "Open a.png" })).toBeVisible()
    expect(screen.getByText("Here's the chart")).toBeInTheDocument()
  })

  it("preview as an image or file name in the sidebar", () => {
    const agents = new Map([["agent-1", makeAgent()]])
    const imageOnly = makeMessage({ body: "", attachments: [img("a")] })
    expect(chatPreview(makeChat({ lastMessage: imageOnly }), agents)).toBe("🖼 Image")
    const fileOnly = makeMessage({
      author: { kind: "user", agentId: null },
      body: "",
      attachments: [pdf],
    })
    expect(chatPreview(makeChat({ lastMessage: fileOnly }), agents)).toBe("You: 📎 report.pdf")
  })
})

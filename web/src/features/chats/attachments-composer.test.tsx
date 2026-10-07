import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Message } from "@/lib/api-client"
import type { Attachment } from "@/lib/attachments"
import { makeChat, makeMessage } from "@/test/fixtures"

import { ChatComposer } from "./chat-view"
import { uploadsStore } from "./uploads-store"

function file(name: string, size = 10, type = "text/plain") {
  const f = new File(["x"], name, { type })
  Object.defineProperty(f, "size", { value: size })
  return f
}

function attachment(id: string, name: string): Attachment {
  return {
    id,
    name,
    mime: "text/plain",
    size: 10,
    url: `/api/attachments/${id}`,
    width: null,
    height: null,
  }
}

/** Upload responses we control: each call waits until released. */
function controlledUploads() {
  const pending: { resolve: (r: Response) => void; name: string }[] = []
  const fetchMock = vi.fn<typeof fetch>((_url, init) => {
    const body = init?.body
    const sent = body instanceof FormData ? body.get("file") : null
    const name = sent instanceof File ? sent.name : ""
    return new Promise<Response>((resolve) => pending.push({ resolve, name }))
  })
  vi.stubGlobal("fetch", fetchMock)
  return {
    fetchMock,
    async finish(index: number, id: string) {
      const item = pending[index]
      if (!item) throw new Error("no upload")
      await act(async () => {
        item.resolve(new Response(JSON.stringify(attachment(id, item.name)), { status: 201 }))
        await Promise.resolve()
      })
    },
    async fail(index: number) {
      await act(async () => {
        pending[index]?.resolve(
          new Response(JSON.stringify({ message: "Disk full" }), { status: 400 }),
        )
        await Promise.resolve()
      })
    },
  }
}

function captureSends() {
  const sent: unknown[] = []
  vi.spyOn(api, "POST").mockImplementation((...args: unknown[]) => {
    const [path, init] = args
    if (path === "/chats/{chatId}/messages" && typeof init === "object" && init && "body" in init) {
      sent.push(init.body)
    }
    const message: Message = makeMessage({ author: { kind: "user", agentId: null } })
    return Promise.resolve({
      data: message,
      error: undefined,
      response: new Response(null, { status: 201 }),
    })
  })
  return sent
}

function renderComposer() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <ChatComposer chat={makeChat()} members={[]} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

const send = () => screen.getByRole("button", { name: "Send message" })
const input = () => screen.getByTestId("attach-input")

beforeEach(() => uploadsStore.setState({ byChat: {}, notice: {} }))
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("attaching files in the composer", () => {
  it("has an Attach files button", () => {
    renderComposer()
    expect(screen.getByRole("button", { name: "Attach files" })).toBeVisible()
  })

  it("uploads added files, keeps Send disabled while uploading, then sends their ids in order", async () => {
    const uploads = controlledUploads()
    const sent = captureSends()
    renderComposer()

    await userEvent.upload(input(), [file("a.txt"), file("b.txt")])
    expect(uploads.fetchMock).toHaveBeenCalledTimes(2)
    expect(screen.getByLabelText("Uploading a.txt")).toBeInTheDocument()
    expect(send()).toHaveAttribute("aria-disabled", "true")

    await uploads.finish(1, "att-b")
    await uploads.finish(0, "att-a")
    await waitFor(() => expect(send()).not.toHaveAttribute("aria-disabled"))

    // Attachments alone are enough to send; Enter follows the same rules.
    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "{Enter}")
    expect(sent).toHaveLength(1)
    expect(sent[0]).toMatchObject({ body: "", attachmentIds: ["att-a", "att-b"] })
    expect(screen.queryByRole("list", { name: "Attachments" })).not.toBeInTheDocument()
  })

  it("removes a file and shows retry on failure", async () => {
    const uploads = controlledUploads()
    renderComposer()
    await userEvent.upload(input(), [file("a.txt"), file("b.txt")])
    await userEvent.click(screen.getByRole("button", { name: "Remove a.txt" }))
    expect(screen.queryByText("a.txt")).not.toBeInTheDocument()

    await uploads.fail(1)
    expect(await screen.findByText("Disk full")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Retry uploading b.txt" }))
    expect(uploads.fetchMock).toHaveBeenCalledTimes(3)
  })

  it("rejects files over 25 MB and more than 10 per message", async () => {
    controlledUploads()
    renderComposer()
    await userEvent.upload(input(), [file("huge.mov", 26 * 1024 * 1024)])
    expect(screen.getByRole("alert")).toHaveTextContent("huge.mov is over 25 MB.")

    await userEvent.upload(
      input(),
      Array.from({ length: 11 }, (_, i) => file(`f${i}.txt`)),
    )
    expect(screen.getByRole("alert")).toHaveTextContent(
      "You can attach up to 10 files per message.",
    )
    expect(screen.getAllByRole("button", { name: /^Remove / })).toHaveLength(10)
  })

  it("attaches pasted files instead of pasting text", async () => {
    const uploads = controlledUploads()
    renderComposer()
    const shot = file("screenshot.png", 100, "image/png")
    fireEvent.paste(screen.getByRole("textbox", { name: "Message" }), {
      clipboardData: { files: [shot], items: [], types: ["Files"] },
    })
    expect(uploads.fetchMock).toHaveBeenCalledTimes(1)
    expect(await screen.findByRole("button", { name: "Remove screenshot.png" })).toBeVisible()
  })
})

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Message } from "@/lib/api-client"
import type { Attachment } from "@/lib/attachments"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent, makeChat, makeMessage } from "@/test/fixtures"

import { ChatComposer } from "./chat-view"
import { pendingStore } from "./pending-store"
import { JumpToMessageContext, QuoteBlock } from "./quote-block"
import { quoteExcerpt, quoteOf, useReplyStore } from "./reply-store"
import { uploadsStore } from "./uploads-store"

const agents = new Map([["agent-1", makeAgent({ id: "agent-1", name: "barn" })]])
const original = makeMessage({
  id: "m-orig",
  chatId: "chat-1",
  author: { kind: "agent", agentId: "agent-1" },
  body: "The **deploy** finished.\n\nAll checks passed.",
})
const image: Attachment = {
  id: "a1",
  name: "graph.png",
  mime: "image/png",
  size: 10,
  url: "/api/attachments/a1",
  width: 10,
  height: 10,
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
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } })
  client.setQueryData(queryKeys.agents, [...agents.values()])
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ChatComposer chat={makeChat({ id: "chat-1" })} members={[]} />
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  useReplyStore.setState({ byChat: {}, focusRequest: 0 })
  uploadsStore.setState({ byChat: {}, notice: {} })
  pendingStore.setState({ byChat: {} })
})
afterEach(() => vi.restoreAllMocks())

describe("replying in the composer", () => {
  it("shows who's being replied to with a plain excerpt, and cancels", async () => {
    renderComposer()
    expect(screen.queryByRole("status", { name: /Replying to/ })).not.toBeInTheDocument()

    useReplyStore.getState().startReply(original)
    const bar = await screen.findByRole("status", { name: "Replying to barn" })
    expect(bar).toHaveTextContent("Replying to barn · The deploy finished. All checks passed.")
    expect(screen.getByRole("textbox", { name: "Message" })).toHaveFocus()

    await userEvent.click(screen.getByRole("button", { name: "Cancel reply" }))
    expect(screen.queryByRole("status", { name: /Replying to/ })).not.toBeInTheDocument()
  })

  it("cancels with Escape in the composer", async () => {
    renderComposer()
    useReplyStore.getState().startReply(original)
    await screen.findByRole("status", { name: "Replying to barn" })
    await userEvent.keyboard("{Escape}")
    expect(screen.queryByRole("status", { name: /Replying to/ })).not.toBeInTheDocument()
  })

  it("sends replyToId, shows the quote on the pending bubble, and clears the reply", async () => {
    const sent = captureSends()
    renderComposer()
    useReplyStore.getState().startReply(original)
    await screen.findByRole("status", { name: "Replying to barn" })

    await userEvent.type(screen.getByRole("textbox", { name: "Message" }), "nice{Enter}")
    expect(sent).toEqual([expect.objectContaining({ body: "nice", replyToId: "m-orig" })])
    expect(pendingStore.getState().byChat["chat-1"]?.[0]?.replyTo).toMatchObject({
      id: "m-orig",
      available: true,
      author: { kind: "agent", agentId: "agent-1" },
    })
    expect(useReplyStore.getState().byChat["chat-1"]).toBeNull()
    expect(screen.queryByRole("status", { name: /Replying to/ })).not.toBeInTheDocument()
  })

  it("keeps replies per chat", () => {
    useReplyStore.getState().startReply(original)
    expect(useReplyStore.getState().byChat["chat-1"]?.id).toBe("m-orig")
    expect(useReplyStore.getState().byChat["other"]).toBeUndefined()
  })
})

describe("QuoteBlock", () => {
  it("shows the author and a plain excerpt, and jumps to the original", async () => {
    const jump = vi.fn<(id: string) => void>()
    render(
      <JumpToMessageContext value={jump}>
        <QuoteBlock quote={quoteOf(original)} agents={agents} align="start" />
      </JumpToMessageContext>,
    )
    const quote = screen.getByRole("button", { name: /^Reply to barn/ })
    expect(quote).toHaveTextContent("barn")
    expect(quote).toHaveTextContent("The deploy finished. All checks passed.")
    expect(quote.textContent).not.toContain("**")
    await userEvent.click(quote)
    expect(jump).toHaveBeenCalledWith("m-orig")
  })

  it("names the user 'You' and shows an image thumbnail for image-only messages", () => {
    const mine = makeMessage({
      author: { kind: "user", agentId: null },
      body: "",
      attachments: [image],
    })
    const { container } = render(<QuoteBlock quote={quoteOf(mine)} agents={agents} align="end" />)
    expect(screen.getByText("You")).toBeVisible()
    expect(screen.getByText("Image")).toBeVisible()
    expect(container.querySelector("img")).toHaveAttribute("src", "/api/attachments/a1")
  })

  it("shows a deleted original as muted text, not a link", () => {
    render(<QuoteBlock quote={{ id: "gone", available: false }} agents={agents} align="start" />)
    expect(screen.getByText("Original message deleted")).toBeVisible()
    expect(screen.queryByRole("button")).not.toBeInTheDocument()
  })
})

describe("quote helpers", () => {
  it("truncates long bodies like the server and describes files", () => {
    const long = makeMessage({ body: "x".repeat(400) })
    expect(quoteOf(long).body).toHaveLength(301)
    expect(
      quoteExcerpt({
        body: "",
        attachments: [{ ...image, name: "r.pdf", mime: "application/pdf" }],
      }),
    ).toBe("r.pdf")
    expect(quoteExcerpt({ body: "", attachments: [image, image] })).toBe("2 images")
    expect(
      quoteExcerpt({
        body: "",
        attachments: [image, { ...image, name: "summary.md", mime: "text/markdown" }],
      }),
    ).toBe("summary.md +1")
  })
})

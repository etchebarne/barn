import { act, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TerminalConnection, terminalUrl } from "./terminal-connection"
import { TerminalSession, withCtrl } from "./terminal-session"

// A stand-in for xterm.js (it needs a real canvas): records input handlers and output.
const terms: FakeTerminal[] = []
let fitSize = { cols: 80, rows: 24 }
class FakeTerminal {
  cols = 80
  rows = 24
  options: Record<string, unknown> = {}
  written: Uint8Array[] = []
  dataHandler: (data: string) => void = () => {}
  constructor() {
    terms.push(this)
  }
  loadAddon(addon: { activate?: (t: FakeTerminal) => void }) {
    addon.activate?.(this)
  }
  open() {}
  onData(handler: (data: string) => void) {
    this.dataHandler = handler
  }
  onBinary() {}
  write(bytes: Uint8Array) {
    this.written.push(bytes)
  }
  focus() {}
  reset() {}
  dispose() {}
}
class FakeFit {
  private term: FakeTerminal | null = null
  activate(term: FakeTerminal) {
    this.term = term
  }
  fit() {
    if (!this.term) return
    this.term.cols = fitSize.cols
    this.term.rows = fitSize.rows
  }
}
vi.mock("@xterm/xterm", () => ({ Terminal: FakeTerminal }))
vi.mock("@xterm/addon-fit", () => ({ FitAddon: FakeFit }))
vi.mock("@xterm/addon-web-links", () => ({
  WebLinksAddon: class {
    activate() {}
  },
}))
vi.mock("@xterm/xterm/css/xterm.css", () => ({}))

class FakeSocket {
  static OPEN = 1
  readyState = 0
  binaryType = ""
  sent: unknown[] = []
  closedWith: number | null = null
  private listeners: Record<string, ((event: unknown) => void)[]> = {}
  readonly url: string
  constructor(url: string) {
    this.url = url
  }
  addEventListener(type: string, listener: (event: unknown) => void) {
    ;(this.listeners[type] ??= []).push(listener)
  }
  emit(type: string, event: unknown = {}) {
    for (const listener of this.listeners[type] ?? []) listener(event)
  }
  open() {
    this.readyState = 1
    this.emit("open")
  }
  send(data: unknown) {
    this.sent.push(data)
  }
  close(code: number) {
    this.closedWith = code
    this.readyState = 3
  }
}

let resize: ResizeObserverCallback | null = null
const OriginalResizeObserver = window.ResizeObserver
beforeEach(() => {
  terms.length = 0
  fitSize = { cols: 80, rows: 24 }
  // The setup's stub is writable, not configurable: swap it for one that keeps the callback.
  window.ResizeObserver = class {
    constructor(callback: ResizeObserverCallback) {
      resize = callback
    }
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  // jsdom doesn't lay out: give the terminal box a size.
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(800)
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(400)
})
afterEach(() => {
  window.ResizeObserver = OriginalResizeObserver
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const decode = (data: unknown) =>
  // TextEncoder's arrays come from another realm than jsdom's Uint8Array.
  ArrayBuffer.isView(data) ? new TextDecoder().decode(data) : String(data)

describe("TerminalSession", () => {
  it("sends keystrokes as binary and resizes as JSON, and shows the shell's output", async () => {
    const sockets: FakeSocket[] = []
    render(
      <TerminalSession
        agentId="research"
        active
        createSocket={(url) => {
          const socket = new FakeSocket(url)
          sockets.push(socket)
          // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a test double
          return socket as unknown as WebSocket
        }}
      />,
    )
    await waitFor(() => expect(sockets).toHaveLength(1))
    const socket = sockets[0]
    const term = terms[0]
    if (!socket || !term) throw new Error("not started")
    expect(socket.url).toBe(
      `ws://${window.location.host}/api/agents/research/sandbox/terminal?cols=80&rows=24`,
    )
    expect(socket.binaryType).toBe("arraybuffer")
    act(() => socket.open())

    act(() => term.dataHandler("ls -la\r"))
    expect(socket.sent).toHaveLength(1)
    expect(ArrayBuffer.isView(socket.sent[0])).toBe(true)
    expect(decode(socket.sent[0])).toBe("ls -la\r")

    fitSize = { cols: 120, rows: 40 }
    act(() => resize?.([], new OriginalResizeObserver(() => {})))
    expect(socket.sent[1]).toBe(JSON.stringify({ type: "resize", cols: 120, rows: 40 }))
    // Same size again: nothing new is sent.
    act(() => resize?.([], new OriginalResizeObserver(() => {})))
    expect(socket.sent).toHaveLength(2)

    act(() =>
      socket.emit("message", {
        data: Uint8Array.from("total 0\r\n", (c) => c.charCodeAt(0)).buffer,
      }),
    )
    expect(decode(term.written[0])).toBe("total 0\r\n")
  })

  it("offers to reconnect when the shell exits", async () => {
    const sockets: FakeSocket[] = []
    render(
      <TerminalSession
        agentId="research"
        active
        createSocket={(url) => {
          const socket = new FakeSocket(url)
          sockets.push(socket)
          // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a test double
          return socket as unknown as WebSocket
        }}
      />,
    )
    await waitFor(() => expect(sockets).toHaveLength(1))
    act(() => sockets[0]?.open())
    act(() => sockets[0]?.emit("close", { code: 1000 }))
    expect(await screen.findByText("Session ended")).toBeVisible()
    await userEvent.click(screen.getByRole("button", { name: "Reconnect" }))
    expect(sockets).toHaveLength(2)
  })

  it("turns the next key into a control character with sticky Ctrl", async () => {
    const sockets: FakeSocket[] = []
    render(
      <TerminalSession
        agentId="research"
        active
        createSocket={(url) => {
          const socket = new FakeSocket(url)
          sockets.push(socket)
          // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a test double
          return socket as unknown as WebSocket
        }}
      />,
    )
    await waitFor(() => expect(sockets).toHaveLength(1))
    act(() => sockets[0]?.open())
    await userEvent.click(screen.getByRole("button", { name: "Ctrl", hidden: true }))
    act(() => terms[0]?.dataHandler("c"))
    expect(decode(sockets[0]?.sent.at(-1))).toBe("\x03")
    await userEvent.click(screen.getByRole("button", { name: "Escape", hidden: true }))
    expect(decode(sockets[0]?.sent.at(-1))).toBe("\x1b")
  })
})

describe("TerminalConnection", () => {
  it("builds a same-origin ws(s) URL", () => {
    expect(terminalUrl("a b", 100, 30, { protocol: "https:", host: "bot.example" })).toBe(
      "wss://bot.example/api/agents/a%20b/sandbox/terminal?cols=100&rows=30",
    )
  })

  it("reports a dropped connection as an error, and closing ends the shell", () => {
    const socket = new FakeSocket("x")
    const onEnd = vi.fn<(end: string) => void>()
    const connection = new TerminalConnection({
      agentId: "a",
      cols: 80,
      rows: 24,
      onOutput: () => {},
      onEnd,
      // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a test double
      createSocket: () => socket as unknown as WebSocket,
    })
    socket.open()
    socket.emit("close", { code: 1006 })
    expect(onEnd).toHaveBeenCalledWith("error")
    connection.close()
    expect(onEnd).toHaveBeenCalledTimes(1)
  })

  it("knows which keys take Ctrl", () => {
    expect(withCtrl("c")).toBe("\x03")
    expect(withCtrl("[")).toBe("\x1b")
    expect(withCtrl("1")).toBe("1")
  })
})

/**
 * One shell in the agent's computer over a WebSocket. Binary frames carry terminal bytes both
 * ways; resizes go as `{"type":"resize","cols":N,"rows":M}` text frames. Kept free of xterm so
 * it's easy to test.
 */

export type TerminalEnd =
  /** The shell exited (the server closed with 1000). */
  | "exited"
  /** The connection failed or dropped. */
  | "error"

export type TerminalConnectionOptions = {
  agentId: string
  cols: number
  rows: number
  onOutput: (bytes: Uint8Array) => void
  onOpen?: () => void
  onEnd: (end: TerminalEnd) => void
  /** Injected for tests. */
  createSocket?: (url: string) => WebSocket
  location?: Pick<Location, "protocol" | "host">
}

export function terminalUrl(
  agentId: string,
  cols: number,
  rows: number,
  location: Pick<Location, "protocol" | "host"> = window.location,
): string {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"
  const query = new URLSearchParams({ cols: String(cols), rows: String(rows) })
  return `${protocol}//${location.host}/api/agents/${encodeURIComponent(agentId)}/sandbox/terminal?${query.toString()}`
}

export class TerminalConnection {
  private socket: WebSocket
  /** The size the shell knows about, and the latest size asked for. */
  private sent: { cols: number; rows: number }
  private size: { cols: number; rows: number }
  private ended = false
  private readonly encoder = new TextEncoder()
  private readonly options: TerminalConnectionOptions

  constructor(options: TerminalConnectionOptions) {
    this.options = options
    const { agentId, cols, rows } = options
    this.size = { cols, rows }
    this.sent = { cols, rows }
    const url = terminalUrl(agentId, cols, rows, options.location)
    this.socket = options.createSocket ? options.createSocket(url) : new WebSocket(url)
    this.socket.binaryType = "arraybuffer"
    this.socket.addEventListener("open", () => {
      // Resized while connecting: catch the shell up.
      this.flushSize()
      options.onOpen?.()
    })
    this.socket.addEventListener("message", (event: MessageEvent<unknown>) => {
      const { data } = event
      if (data instanceof ArrayBuffer) options.onOutput(new Uint8Array(data))
      else if (typeof data === "string") options.onOutput(this.encoder.encode(data))
    })
    this.socket.addEventListener("close", (event: CloseEvent) =>
      this.end(event.code === 1000 ? "exited" : "error"),
    )
    this.socket.addEventListener("error", () => this.end("error"))
  }

  private end(end: TerminalEnd) {
    if (this.ended) return
    this.ended = true
    this.options.onEnd(end)
  }

  /** Keystrokes (or pasted text) as UTF-8 bytes. */
  send(data: string | Uint8Array<ArrayBuffer>) {
    if (this.socket.readyState !== WebSocket.OPEN) return
    this.socket.send(typeof data === "string" ? this.encoder.encode(data) : data)
  }

  /** Tells the shell its new size; only when it changed. */
  resize(cols: number, rows: number) {
    this.size = { cols, rows }
    this.flushSize()
  }

  private flushSize() {
    const { cols, rows } = this.size
    if (cols === this.sent.cols && rows === this.sent.rows) return
    if (this.socket.readyState !== WebSocket.OPEN) return
    this.sent = { cols, rows }
    this.socket.send(JSON.stringify({ type: "resize", cols, rows }))
  }

  /** Ends the shell (the server stops it when the socket closes). */
  close() {
    this.ended = true
    if (
      this.socket.readyState === WebSocket.CONNECTING ||
      this.socket.readyState === WebSocket.OPEN
    ) {
      this.socket.close(1000)
    }
  }
}

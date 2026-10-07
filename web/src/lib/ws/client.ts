import type { WsEvent } from "../api-client"
import { useConnectionStore } from "./status"

export type RealtimeOptions = {
  /** Defaults to `/api/ws` on the current origin. */
  url?: string
  onEvent: (event: WsEvent) => void
  /** Called after a connection drops and is re-established (state may have been missed). */
  onReconnect?: () => void
  /** Injected for tests. */
  createSocket?: (url: string) => WebSocket
}

const BASE_DELAY_MS = 500
const MAX_DELAY_MS = 30_000

/** Exponential backoff with "equal jitter": half fixed, half random. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const exp = Math.min(MAX_DELAY_MS, BASE_DELAY_MS * 2 ** attempt)
  return Math.round(exp / 2 + random() * (exp / 2))
}

export function defaultWsUrl(
  location: Pick<Location, "protocol" | "host"> = window.location,
): string {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"
  return `${protocol}//${location.host}/api/ws`
}

const KNOWN_EVENTS = new Set([
  "message.created",
  "message.updated",
  "agent.activity",
  "agent.created",
  "agent.updated",
  "agent.deleted",
  "chat.created",
  "chat.read",
  "chat.cleared",
  "sidebar.updated",
])

export function parseWsEvent(data: unknown): WsEvent | null {
  if (typeof data !== "string") return null
  try {
    const parsed: unknown = JSON.parse(data)
    if (
      parsed &&
      typeof parsed === "object" &&
      "type" in parsed &&
      typeof parsed.type === "string" &&
      KNOWN_EVENTS.has(parsed.type)
    ) {
      // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- discriminator checked above
      return parsed as WsEvent
    }
  } catch {
    // Ignore malformed frames.
  }
  return null
}

/**
 * The app's single WebSocket connection. Reconnects with backoff, and immediately when the
 * browser comes back online or the tab becomes visible again.
 */
export class RealtimeClient {
  private socket: WebSocket | null = null
  private attempt = 0
  private timer: ReturnType<typeof setTimeout> | null = null
  private stopped = true
  private hasConnected = false
  private readonly url: string
  private readonly options: RealtimeOptions

  constructor(options: RealtimeOptions) {
    this.options = options
    this.url = options.url ?? defaultWsUrl()
  }

  start() {
    if (!this.stopped) return
    this.stopped = false
    window.addEventListener("online", this.reconnectNow)
    document.addEventListener("visibilitychange", this.onVisibility)
    this.connect()
  }

  stop() {
    this.stopped = true
    window.removeEventListener("online", this.reconnectNow)
    document.removeEventListener("visibilitychange", this.onVisibility)
    this.clearTimer()
    const socket = this.socket
    this.socket = null
    socket?.close()
    useConnectionStore.getState().setStatus("idle")
  }

  private connect() {
    this.clearTimer()
    const setStatus = useConnectionStore.getState().setStatus
    setStatus(this.hasConnected ? "reconnecting" : "connecting")

    const socket = (this.options.createSocket ?? ((url) => new WebSocket(url)))(this.url)
    this.socket = socket

    socket.addEventListener("open", () => {
      if (this.socket !== socket) return
      const isReconnect = this.hasConnected
      this.hasConnected = true
      this.attempt = 0
      setStatus("open")
      if (isReconnect) this.options.onReconnect?.()
    })
    socket.addEventListener("message", (message: MessageEvent) => {
      if (this.socket !== socket) return
      const event = parseWsEvent(message.data)
      if (event) this.options.onEvent(event)
    })
    socket.addEventListener("close", () => {
      if (this.socket !== socket) return
      this.socket = null
      if (this.stopped) return
      setStatus(this.hasConnected ? "reconnecting" : "connecting")
      this.timer = setTimeout(() => this.connect(), backoffDelay(this.attempt))
      this.attempt += 1
    })
  }

  private readonly reconnectNow = () => {
    if (this.stopped || this.socket) return
    this.attempt = 0
    this.connect()
  }

  private readonly onVisibility = () => {
    if (document.visibilityState === "visible") this.reconnectNow()
  }

  private clearTimer() {
    if (this.timer !== null) {
      clearTimeout(this.timer)
      this.timer = null
    }
  }
}

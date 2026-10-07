import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { WsEvent } from "../api-client"
import { backoffDelay, defaultWsUrl, parseWsEvent, RealtimeClient } from "./client"
import { useConnectionStore } from "./status"

/** Minimal controllable WebSocket stand-in. */
class FakeSocket extends EventTarget {
  closed = false
  open() {
    this.dispatchEvent(new Event("open"))
  }
  receive(data: unknown) {
    this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify(data) }))
  }
  drop() {
    this.dispatchEvent(new Event("close"))
  }
  close() {
    this.closed = true
    this.dispatchEvent(new Event("close"))
  }
}

describe("backoffDelay", () => {
  it("grows exponentially and is capped at 30s", () => {
    expect(backoffDelay(0, () => 1)).toBe(500)
    expect(backoffDelay(3, () => 1)).toBe(4000)
    expect(backoffDelay(20, () => 1)).toBe(30_000)
  })

  it("keeps at least half the delay when jittering", () => {
    expect(backoffDelay(3, () => 0)).toBe(2000)
  })
})

describe("parseWsEvent", () => {
  it("accepts known events and rejects junk", () => {
    expect(parseWsEvent('{"type":"chat.read","chatId":"c","lastMessageId":"m"}')).toEqual({
      type: "chat.read",
      chatId: "c",
      lastMessageId: "m",
    })
    expect(parseWsEvent('{"type":"nope"}')).toBeNull()
    expect(parseWsEvent("not json")).toBeNull()
    expect(parseWsEvent(new ArrayBuffer(2))).toBeNull()
  })
})

describe("defaultWsUrl", () => {
  it("uses the same origin with ws/wss", () => {
    expect(defaultWsUrl({ protocol: "https:", host: "barn.example:8443" })).toBe(
      "wss://barn.example:8443/api/ws",
    )
    expect(defaultWsUrl({ protocol: "http:", host: "localhost:5173" })).toBe(
      "ws://localhost:5173/api/ws",
    )
  })
})

describe("RealtimeClient", () => {
  let sockets: FakeSocket[]

  beforeEach(() => {
    vi.useFakeTimers()
    sockets = []
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  function setup() {
    const onEvent = vi.fn<(event: WsEvent) => void>()
    const onReconnect = vi.fn<() => void>()
    const client = new RealtimeClient({
      url: "ws://test/api/ws",
      onEvent,
      onReconnect,
      createSocket: () => {
        const socket = new FakeSocket()
        sockets.push(socket)
        // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- test double
        return socket as unknown as WebSocket
      },
    })
    return { client, onEvent, onReconnect }
  }

  it("delivers parsed events and tracks connection status", () => {
    const { client, onEvent } = setup()
    client.start()
    expect(useConnectionStore.getState().status).toBe("connecting")

    sockets[0]?.open()
    expect(useConnectionStore.getState().status).toBe("open")

    sockets[0]?.receive({ type: "chat.read", chatId: "c", lastMessageId: "m" })
    expect(onEvent).toHaveBeenCalledWith({ type: "chat.read", chatId: "c", lastMessageId: "m" })
    client.stop()
  })

  it("reconnects with backoff after a drop and asks for a resync", () => {
    const { client, onReconnect } = setup()
    client.start()
    sockets[0]?.open()

    sockets[0]?.drop()
    expect(useConnectionStore.getState().status).toBe("reconnecting")
    expect(sockets).toHaveLength(1)

    vi.advanceTimersByTime(30_000)
    expect(sockets).toHaveLength(2)
    expect(onReconnect).not.toHaveBeenCalled()

    sockets[1]?.open()
    expect(onReconnect).toHaveBeenCalledTimes(1)
    expect(useConnectionStore.getState().status).toBe("open")
    client.stop()
  })

  it("stops reconnecting once stopped", () => {
    const { client } = setup()
    client.start()
    sockets[0]?.open()
    client.stop()

    expect(sockets[0]?.closed).toBe(true)
    vi.advanceTimersByTime(60_000)
    expect(sockets).toHaveLength(1)
    expect(useConnectionStore.getState().status).toBe("idle")
  })
})

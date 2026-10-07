import type { FitAddon } from "@xterm/addon-fit"
import type { ITheme, Terminal } from "@xterm/xterm"
import { cn } from "cn"
import { useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { useThemeStore } from "@/lib/theme"

import { TerminalConnection, type TerminalEnd } from "./terminal-connection"

export const MONO_FONT =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace'

/** Matches the app's neutral palette (--background / --foreground in each theme). */
const THEMES: Record<"light" | "dark", ITheme> = {
  dark: {
    background: "#0a0a0a",
    foreground: "#fafafa",
    cursor: "#fafafa",
    cursorAccent: "#0a0a0a",
    selectionBackground: "#404040",
    black: "#262626",
    brightBlack: "#737373",
    white: "#d4d4d4",
    brightWhite: "#fafafa",
  },
  light: {
    background: "#ffffff",
    foreground: "#0a0a0a",
    cursor: "#0a0a0a",
    cursorAccent: "#ffffff",
    selectionBackground: "#d4d4d4",
    black: "#0a0a0a",
    brightBlack: "#525252",
    white: "#a3a3a3",
    brightWhite: "#d4d4d4",
    yellow: "#a16207",
    brightYellow: "#ca8a04",
  },
}

type Xterm = {
  Terminal: typeof Terminal
  FitAddon: typeof FitAddon
  WebLinksAddon: typeof import("@xterm/addon-web-links").WebLinksAddon
}

let xterm: Promise<Xterm> | null = null

/** xterm.js loads on first use, so it stays out of the main bundle. */
export function loadXterm(): Promise<Xterm> {
  xterm ??= Promise.all([
    import("@xterm/xterm"),
    import("@xterm/addon-fit"),
    import("@xterm/addon-web-links"),
    import("@xterm/xterm/css/xterm.css"),
  ]).then(([core, fit, links]) => ({
    Terminal: core.Terminal,
    FitAddon: fit.FitAddon,
    WebLinksAddon: links.WebLinksAddon,
  }))
  return xterm
}

type Phase = "loading" | "connecting" | "open" | TerminalEnd

/** Keys a phone keyboard doesn't have, sent as terminal input. */
const EXTRA_KEYS: { label: string; aria: string; data: string }[] = [
  { label: "Esc", aria: "Escape", data: "\x1b" },
  { label: "Tab", aria: "Tab", data: "\t" },
  { label: "↑", aria: "Up", data: "\x1b[A" },
  { label: "↓", aria: "Down", data: "\x1b[B" },
  { label: "←", aria: "Left", data: "\x1b[D" },
  { label: "→", aria: "Right", data: "\x1b[C" },
  { label: "|", aria: "Pipe", data: "|" },
  { label: "~", aria: "Tilde", data: "~" },
  { label: "/", aria: "Slash", data: "/" },
]

/** With sticky Ctrl, a letter (or one of @[\]^_) becomes its control character. */
export function withCtrl(data: string): string {
  if (data.length !== 1) return data
  const code = data.toUpperCase().charCodeAt(0)
  return code >= 64 && code <= 95 ? String.fromCharCode(code & 0x1f) : data
}

/**
 * One shell: xterm.js in a box that fits its container (resizes are sent to the shell), with
 * an overlay to reconnect when the shell exits or the connection drops.
 */
export function TerminalSession({
  agentId,
  active,
  createSocket,
}: {
  agentId: string
  /** The visible session; it takes focus when shown. */
  active: boolean
  createSocket?: (url: string) => WebSocket
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const connectionRef = useRef<TerminalConnection | null>(null)
  const ctrlRef = useRef(false)
  const [ctrl, setCtrl] = useState(false)
  const [phase, setPhase] = useState<Phase>("loading")
  const theme = useThemeStore((s) => s.resolved)
  const themeRef = useRef(theme)
  const socketFactory = useRef(createSocket)

  function connect() {
    const term = termRef.current
    if (!term) return
    connectionRef.current?.close()
    setPhase("connecting")
    connectionRef.current = new TerminalConnection({
      agentId,
      cols: term.cols,
      rows: term.rows,
      createSocket: socketFactory.current,
      onOpen: () => setPhase("open"),
      onOutput: (bytes) => term.write(bytes),
      onEnd: (end) => setPhase(end),
    })
  }

  function send(data: string) {
    connectionRef.current?.send(data)
  }

  // Create the terminal once; the connection follows it.
  useEffect(() => {
    let disposed = false
    let observer: ResizeObserver | null = null
    void loadXterm().then(({ Terminal, FitAddon, WebLinksAddon }) => {
      const container = containerRef.current
      if (disposed || !container) return
      const term = new Terminal({
        cursorBlink: true,
        fontFamily: MONO_FONT,
        fontSize: 13,
        lineHeight: 1.15,
        scrollback: 5000,
        allowProposedApi: false,
        theme: THEMES[themeRef.current],
      })
      const fit = new FitAddon()
      term.loadAddon(fit)
      term.loadAddon(new WebLinksAddon())
      term.open(container)
      termRef.current = term
      fitRef.current = fit
      const refit = () => {
        // Hidden tabs measure 0×0: keep the last size.
        if (container.clientWidth === 0 || container.clientHeight === 0) return
        fit.fit()
        connectionRef.current?.resize(term.cols, term.rows)
      }
      refit()
      term.onData((data) => {
        if (ctrlRef.current) {
          ctrlRef.current = false
          setCtrl(false)
          send(withCtrl(data))
          return
        }
        send(data)
      })
      term.onBinary((data) =>
        connectionRef.current?.send(Uint8Array.from(data, (c) => c.charCodeAt(0))),
      )
      observer = new ResizeObserver(refit)
      observer.observe(container)
      connect()
    })
    return () => {
      disposed = true
      observer?.disconnect()
      connectionRef.current?.close()
      connectionRef.current = null
      termRef.current?.dispose()
      termRef.current = null
    }
    // Once per session; `connect` and `send` only read refs.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId])

  useEffect(() => {
    themeRef.current = theme
    if (termRef.current) termRef.current.options.theme = THEMES[theme]
  }, [theme])

  useEffect(() => {
    if (active && phase === "open") termRef.current?.focus()
  }, [active, phase])

  const ended = phase === "exited" || phase === "error"

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="relative min-h-0 flex-1 bg-[#ffffff] p-2 dark:bg-[#0a0a0a]">
        <div ref={containerRef} className="h-full w-full" data-testid="terminal" />
        {(phase === "loading" || phase === "connecting") && (
          <div className="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 text-sm text-muted-foreground">
            <Spinner />
            {phase === "loading" ? "Loading terminal…" : "Connecting…"}
          </div>
        )}
        {ended && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/70">
            <div
              role="status"
              className="flex flex-col items-center gap-3 rounded-xl border bg-popover p-4 text-center text-sm shadow-lg"
            >
              <p className="font-medium">
                {phase === "exited" ? "Session ended" : "Couldn't connect to the terminal"}
              </p>
              <Button
                size="sm"
                onClick={() => {
                  termRef.current?.reset()
                  connect()
                }}
              >
                {phase === "exited" ? "Reconnect" : "Retry"}
              </Button>
            </div>
          </div>
        )}
      </div>
      {/* Touch screens: the keys a phone keyboard lacks. Pressing doesn't move focus, so the
          keyboard stays up. */}
      <div
        role="toolbar"
        aria-label="Extra keys"
        className="hidden shrink-0 gap-1 overflow-x-auto border-t bg-muted/50 p-1.5 pb-[max(0.375rem,env(safe-area-inset-bottom))] [@media(hover:none)]:flex"
      >
        <Button
          size="sm"
          variant={ctrl ? "default" : "outline"}
          aria-pressed={ctrl}
          className="min-w-11 font-mono"
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => {
            ctrlRef.current = !ctrlRef.current
            setCtrl(ctrlRef.current)
          }}
        >
          Ctrl
        </Button>
        {EXTRA_KEYS.map((key) => (
          <Button
            key={key.aria}
            size="sm"
            variant="outline"
            aria-label={key.aria}
            className={cn("min-w-11 font-mono")}
            onPointerDown={(event) => event.preventDefault()}
            onClick={() => send(key.data)}
          >
            {key.label}
          </Button>
        ))}
      </div>
    </div>
  )
}

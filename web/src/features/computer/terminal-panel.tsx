import { cn } from "cn"
import { PlusIcon, TerminalIcon, XIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"

import { TerminalSession } from "./terminal-session"

type Session = { id: number; label: string }

/**
 * Terminals as small tabs: "+" opens another shell, × closes one (ending its shell). All stay
 * mounted, so switching tabs keeps each shell running.
 */
export function TerminalPanel({
  agentId,
  visible,
  createSocket,
}: {
  agentId: string
  /** Whether the Terminal tab is showing (the active shell takes focus then). */
  visible: boolean
  createSocket?: (url: string) => WebSocket
}) {
  const [sessions, setSessions] = useState<Session[]>([{ id: 1, label: "Shell 1" }])
  const [activeId, setActiveId] = useState(1)
  const [next, setNext] = useState(2)

  function open() {
    const session = { id: next, label: `Shell ${next}` }
    setSessions((list) => [...list, session])
    setActiveId(session.id)
    setNext((n) => n + 1)
  }

  function close(id: number) {
    const index = sessions.findIndex((s) => s.id === id)
    const rest = sessions.filter((s) => s.id !== id)
    setSessions(rest)
    if (id === activeId) setActiveId(rest[Math.max(0, index - 1)]?.id ?? 0)
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1">
        <div
          role="tablist"
          aria-label="Terminals"
          className="flex min-w-0 items-center gap-1 overflow-x-auto"
        >
          {sessions.map((session) => (
            <div
              key={session.id}
              className={cn(
                "flex shrink-0 items-center rounded-md text-xs",
                session.id === activeId ? "bg-muted text-foreground" : "text-muted-foreground",
              )}
            >
              <button
                type="button"
                role="tab"
                aria-selected={session.id === activeId}
                className="flex h-7 items-center gap-1.5 rounded-md pr-1 pl-2 outline-none select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50"
                onClick={() => setActiveId(session.id)}
              >
                <TerminalIcon className="size-3.5" aria-hidden="true" />
                {session.label}
              </button>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Close ${session.label}`}
                className="mr-0.5 text-muted-foreground hover:text-foreground"
                onClick={() => close(session.id)}
              >
                <XIcon />
              </Button>
            </div>
          ))}
        </div>
        <Button variant="ghost" size="icon-xs" aria-label="New terminal" onClick={open}>
          <PlusIcon />
        </Button>
      </div>
      <div className="relative min-h-0 flex-1">
        {sessions.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
            <p>No terminal open.</p>
            <Button size="sm" variant="outline" onClick={open}>
              <PlusIcon />
              New terminal
            </Button>
          </div>
        ) : (
          sessions.map((session) => (
            <div
              key={session.id}
              role="tabpanel"
              aria-label={session.label}
              hidden={session.id !== activeId}
              className="absolute inset-0"
            >
              <TerminalSession
                agentId={agentId}
                active={visible && session.id === activeId}
                createSocket={createSocket}
              />
            </div>
          ))
        )}
      </div>
    </div>
  )
}

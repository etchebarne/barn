import { Link } from "@tanstack/react-router"
import { MonitorIcon, RotateCcwIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { SandboxStatusText } from "@/components/sandbox-status"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { Agent } from "@/lib/api-client"
import { RESTART_COPY, useRestartSandbox, useSandbox } from "@/lib/sandbox"

import { InlineConfirm } from "./settings-sections"

/** The agent's computer (Docker sandbox): its state, a way in, and Restart. */
export function ComputerSection({ agent, onOpen }: { agent: Agent; onOpen: () => void }) {
  const { data: sandbox, isPending, error } = useSandbox(agent.id)
  const restart = useRestartSandbox(agent.id)
  const [confirming, setConfirming] = useState(false)

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-computer">
      <h3 id="agent-computer" className="text-sm font-medium">
        Computer
      </h3>
      {isPending ? (
        <Skeleton className="h-12 w-full rounded-lg" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load its computer: {error.message}</p>
      ) : sandbox.status === "unavailable" ? (
        <p className="text-sm text-muted-foreground">
          Docker isn't available on this server, so {agent.name} has no computer of its own.
        </p>
      ) : confirming ? (
        <InlineConfirm
          tone="warning"
          title={`Restart ${agent.name}'s computer?`}
          confirmLabel="Restart"
          pending={restart.isPending}
          error={restart.error?.message}
          onConfirm={() =>
            restart.mutate(undefined, {
              onSuccess: () => {
                setConfirming(false)
                toast.success(`Restarted ${agent.name}'s computer`)
              },
            })
          }
          onCancel={() => {
            setConfirming(false)
            restart.reset()
          }}
        >
          {RESTART_COPY}
        </InlineConfirm>
      ) : (
        <div className="flex items-center gap-3 rounded-lg bg-muted/50 py-2 pr-2 pl-3 text-sm">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-muted-foreground">
              Its own Linux machine: files in /home/agent, and a terminal.
            </span>
            <SandboxStatusText status={sandbox.status} className="text-xs text-muted-foreground" />
          </div>
          {sandbox.status === "running" && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Restart computer"
              onClick={() => setConfirming(true)}
            >
              <RotateCcwIcon />
            </Button>
          )}
          <Link
            to="/agents/$agentId/computer"
            params={{ agentId: agent.id }}
            className={buttonVariants({ variant: "outline", size: "sm" })}
            onClick={onOpen}
          >
            <MonitorIcon aria-hidden="true" />
            Open computer
          </Link>
        </div>
      )}
    </section>
  )
}

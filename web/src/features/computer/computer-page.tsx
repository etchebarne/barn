import { cn } from "cn"
import { FolderIcon, MonitorOffIcon, RotateCcwIcon, TerminalIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { SandboxStatusText } from "@/components/sandbox-status"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { AgentAvatar, useAgentsById } from "@/features/agents"
import { RESTART_COPY, useRestartSandbox, useSandbox } from "@/lib/sandbox"
import { readStorage, writeStorage } from "@/lib/storage"

import { FilesPanel } from "./files-panel"
import { TerminalPanel } from "./terminal-panel"

type Tab = "files" | "terminal"
const tabKey = (agentId: string) => `computer:tab:${agentId}`

function RestartButton({ agentId, name }: { agentId: string; name: string }) {
  const restart = useRestartSandbox(agentId)
  const [confirming, setConfirming] = useState(false)
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={restart.isPending}
        onClick={() => setConfirming(true)}
      >
        {restart.isPending ? <Spinner /> : <RotateCcwIcon />}
        <span className="max-sm:sr-only">Restart</span>
      </Button>
      <Dialog open={confirming} onOpenChange={(open) => !restart.isPending && setConfirming(open)}>
        <DialogContent showCloseButton={false} role="alertdialog">
          <DialogHeader>
            <DialogTitle>Restart {name}'s computer?</DialogTitle>
            <DialogDescription>{RESTART_COPY}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              autoFocus
              disabled={restart.isPending}
              onClick={() => setConfirming(false)}
            >
              Cancel
            </Button>
            <Button
              disabled={restart.isPending}
              onClick={() =>
                restart.mutate(undefined, {
                  onSuccess: () => {
                    setConfirming(false)
                    toast.success(`Restarted ${name}'s computer`)
                  },
                  onError: (error) => toast.error(`Couldn't restart: ${error.message}`),
                })
              }
            >
              {restart.isPending && <Spinner />}
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function TabButton({
  selected,
  icon,
  children,
  onSelect,
  controls,
}: {
  selected: boolean
  icon: ReactNode
  children: ReactNode
  onSelect: () => void
  controls: string
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={selected}
      aria-controls={controls}
      className={cn(
        "flex h-9 items-center gap-1.5 border-b-2 px-3 text-sm outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50 [&_svg]:size-4",
        selected
          ? "border-foreground font-medium text-foreground"
          : "border-transparent text-muted-foreground hover:text-foreground",
      )}
      onClick={onSelect}
    >
      {icon}
      {children}
    </button>
  )
}

/**
 * An agent's computer (its Docker sandbox): browse and edit its files, or use a real
 * terminal. Both tabs stay mounted, so switching keeps the shell running.
 */
export function ComputerPage({
  agentId,
  createSocket,
}: {
  agentId: string
  /** Injected for tests. */
  createSocket?: (url: string) => WebSocket
}) {
  const agents = useAgentsById()
  const agent = agents.get(agentId)
  const sandbox = useSandbox(agentId)
  const [tab, setTab] = useState<Tab>(() =>
    readStorage(tabKey(agentId)) === "terminal" ? "terminal" : "files",
  )
  // The first shell starts when the Terminal tab is first shown, then stays.
  const [terminalMounted, setTerminalMounted] = useState(tab === "terminal")
  const name = agent?.name ?? "Agent"
  const status = sandbox.data?.status
  const shared = (sandbox.data?.sharedWith ?? [])
    .map((id) => agents.get(id)?.name)
    .filter((n): n is string => !!n)

  function select(next: Tab) {
    setTab(next)
    writeStorage(tabKey(agentId), next)
    if (next === "terminal") setTerminalMounted(true)
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
      <PageHeader
        actions={
          status && status !== "unavailable" ? (
            <RestartButton agentId={agentId} name={name} />
          ) : undefined
        }
      >
        <AgentAvatar id={agent?.id} name={name} />
        <div className="flex min-w-0 flex-1 flex-col">
          <h1 className="truncate text-sm leading-tight font-medium">{name}'s computer</h1>
          <span className="flex min-w-0 items-center gap-2 truncate text-xs text-muted-foreground">
            {status ? <SandboxStatusText status={status} /> : <span>…</span>}
            {shared.length > 0 && (
              <span className="truncate">· Shared with {shared.join(", ")}</span>
            )}
          </span>
        </div>
      </PageHeader>
      {sandbox.isPending ? (
        <div className="flex flex-col gap-2 p-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-6 w-full" />
          <Skeleton className="h-6 w-3/4" />
        </div>
      ) : sandbox.error ? (
        <p className="p-6 text-sm text-destructive">
          Couldn't load {name}'s computer: {sandbox.error.message}
        </p>
      ) : status === "unavailable" ? (
        <div className="m-auto flex max-w-sm flex-col items-center gap-3 p-6 text-center">
          <MonitorOffIcon className="size-8 text-muted-foreground" aria-hidden="true" />
          <h2 className="font-medium">No computer on this server</h2>
          <p className="text-sm text-muted-foreground">
            Docker isn't available on this server, so agents can't have their own computer. Once
            Docker is installed and running, {name}'s files and terminal show up here.
          </p>
        </div>
      ) : (
        <>
          <div role="tablist" aria-label="Computer" className="flex shrink-0 gap-1 border-b px-2">
            <TabButton
              selected={tab === "files"}
              controls="computer-files"
              icon={<FolderIcon />}
              onSelect={() => select("files")}
            >
              Files
            </TabButton>
            <TabButton
              selected={tab === "terminal"}
              controls="computer-terminal"
              icon={<TerminalIcon />}
              onSelect={() => select("terminal")}
            >
              Terminal
            </TabButton>
          </div>
          <div className="relative min-h-0 flex-1">
            <div
              id="computer-files"
              role="tabpanel"
              aria-label="Files"
              hidden={tab !== "files"}
              className="absolute inset-0"
            >
              <FilesPanel agentId={agentId} />
            </div>
            <div
              id="computer-terminal"
              role="tabpanel"
              aria-label="Terminal"
              hidden={tab !== "terminal"}
              className="absolute inset-0"
            >
              {terminalMounted && (
                <TerminalPanel
                  agentId={agentId}
                  visible={tab === "terminal"}
                  createSocket={createSocket}
                />
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}

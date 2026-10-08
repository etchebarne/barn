import { CircleCheckIcon, CircleMinusIcon, DownloadIcon, UploadIcon } from "lucide-react"
import { useRef, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { useAgents } from "@/features/agents"

import {
  parseBackup,
  planRestore,
  useExportAgents,
  useRestoreAgents,
  type AgentsBackup,
  type RestoreResult,
} from "./backup"
import { SettingsSection } from "./settings-section"

function RestoreList({ items }: { items: { name: string; ok: boolean; notes: string[] }[] }) {
  return (
    <ul className="flex flex-col gap-2">
      {items.map((item, i) => (
        // The same name can appear twice in a file.
        // oxlint-disable-next-line react/no-array-index-key -- names aren't unique
        <li key={i} className="flex gap-2 text-sm">
          {item.ok ? (
            <CircleCheckIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
          ) : (
            <CircleMinusIcon
              aria-hidden="true"
              className="mt-0.5 size-4 shrink-0 text-muted-foreground"
            />
          )}
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className={item.ok ? "font-medium" : "text-muted-foreground"}>{item.name}</span>
            {item.notes.map((note) => (
              <span key={note} className="text-xs text-muted-foreground">
                {note}
              </span>
            ))}
          </div>
        </li>
      ))}
    </ul>
  )
}

/**
 * Backing up agents: a file with every agent's settings, instructions, personality, memories,
 * tasks, connections (by name) and standing approvals. Restoring shows what it will do first;
 * it only adds agents whose names are free, so it never overwrites one.
 */
export function BackupSection() {
  const exportAgents = useExportAgents()
  const restore = useRestoreAgents()
  const { data: agents = [] } = useAgents()
  const input = useRef<HTMLInputElement>(null)
  const [pending, setPending] = useState<{ file: string; backup: AgentsBackup } | null>(null)
  const [result, setResult] = useState<RestoreResult | null>(null)
  const [problem, setProblem] = useState<string | null>(null)

  const plan = pending
    ? planRestore(
        pending.backup,
        agents.map((a) => a.name),
      )
    : []
  const restorable = plan.filter((p) => p.restore).length

  async function pick(file: File) {
    setResult(null)
    const parsed = parseBackup(await file.text())
    if (typeof parsed === "string") {
      setProblem(parsed)
      setPending(null)
      return
    }
    setProblem(null)
    setPending({ file: file.name, backup: parsed })
  }

  return (
    <SettingsSection
      id="backup"
      title="Back up agents"
      description="A file with every agent's settings, instructions, personality, memories, tasks, connections and standing approvals. It leaves out secrets, chats, files and skills."
    >
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={exportAgents.isPending}
          onClick={() =>
            exportAgents.mutate(undefined, {
              onSuccess: (count) =>
                toast.success(`Downloaded a backup of ${count} agent${count === 1 ? "" : "s"}`),
              onError: (error) => toast.error(`Couldn't back up: ${error.message}`),
            })
          }
        >
          {exportAgents.isPending ? <Spinner /> : <DownloadIcon />}
          Download backup
        </Button>
        <Button variant="ghost" onClick={() => input.current?.click()}>
          <UploadIcon />
          Restore from a backup…
        </Button>
        <input
          ref={input}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0]
            event.target.value = ""
            if (file) void pick(file)
          }}
        />
      </div>

      {problem && (
        <p role="alert" className="text-sm text-destructive">
          {problem}
        </p>
      )}

      {pending && (
        // Nested radius: container radius = button radius (--radius-lg) + padding (p-3).
        <div className="flex flex-col gap-3 rounded-[calc(var(--radius-lg)+0.75rem)] border p-3">
          <p className="text-sm">
            <span className="font-medium">{pending.file}</span>{" "}
            <span className="text-muted-foreground">
              ({pending.backup.agents.length} agent{pending.backup.agents.length === 1 ? "" : "s"})
            </span>
          </p>
          <RestoreList
            items={plan.map((p) => ({
              name: p.name,
              ok: p.restore,
              notes: p.restore ? [] : ["Skipped: an agent with this name already exists."],
            }))}
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setPending(null)}>
              Cancel
            </Button>
            <Button
              disabled={restorable === 0 || restore.isPending}
              onClick={() =>
                restore.mutate(pending.backup, {
                  onSuccess: (res) => {
                    setPending(null)
                    setResult(res)
                  },
                  onError: (error) => toast.error(`Couldn't restore: ${error.message}`),
                })
              }
            >
              {restore.isPending && <Spinner />}
              Restore {restorable} agent{restorable === 1 ? "" : "s"}
            </Button>
          </div>
        </div>
      )}

      {result && (
        <div className="flex flex-col gap-3 rounded-[calc(var(--radius-lg)+0.75rem)] border p-3">
          <p className="text-sm font-medium">
            Restored {result.agents.filter((a) => a.restored).length} of {result.agents.length}
          </p>
          <RestoreList
            items={result.agents.map((a) => ({ name: a.name, ok: a.restored, notes: a.notes }))}
          />
        </div>
      )}
    </SettingsSection>
  )
}

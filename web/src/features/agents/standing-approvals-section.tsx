import { cn } from "cn"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import type { Agent } from "@/lib/api-client"

import { useRemoveStandingApproval, useStandingApprovals } from "./api"

/** "Allowed without asking": standing approvals, each removable. */
export function StandingApprovalsSection({ agent }: { agent: Agent }) {
  const { data: approvals, isPending, error } = useStandingApprovals(agent.id)
  const remove = useRemoveStandingApproval(agent.id)
  const trusted = agent.trustMode === "trusted"

  return (
    <section aria-labelledby="agent-standing-approvals" className="flex flex-col gap-2">
      <h3 id="agent-standing-approvals" className="text-sm font-medium">
        Allowed without asking
      </h3>
      {trusted && (
        <p className="text-sm text-muted-foreground">
          Trusted mode is on, so {agent.name} already skips all approvals. These apply if you turn
          it off.
        </p>
      )}
      <div className={cn("flex flex-col gap-2", trusted && "opacity-60")}>
        {isPending ? (
          <Skeleton className="h-9 w-full rounded-lg" />
        ) : error ? (
          <p className="text-sm text-destructive">Couldn't load approvals: {error.message}</p>
        ) : approvals.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Nothing yet. When {agent.name} asks for approval, choose Always allow, or tell it what
            it can do without asking.
          </p>
        ) : (
          <ul
            className="flex flex-col gap-1"
            aria-label={`What ${agent.name} may do without asking`}
          >
            {approvals.map((approval) => {
              const removing = remove.isPending && remove.variables === approval.id
              return (
                <li
                  key={approval.id}
                  className="flex items-center gap-2 rounded-lg bg-muted/50 py-1.5 pr-1.5 pl-3 text-sm"
                >
                  <span className="min-w-0 flex-1 wrap-break-word">{approval.label}</span>
                  <Button
                    variant="ghost"
                    size="xs"
                    className="shrink-0 text-muted-foreground hover:text-destructive"
                    aria-label={`Remove: ${approval.label}`}
                    disabled={removing}
                    onClick={() =>
                      remove.mutate(approval.id, {
                        onSuccess: () => toast.success(`${agent.name} will ask again`),
                        onError: (e) => toast.error(`Couldn't remove it: ${e.message}`),
                      })
                    }
                  >
                    {removing && <Spinner />}
                    Remove
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </section>
  )
}

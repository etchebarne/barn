import { Trash2Icon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { Agent } from "@/lib/api-client"

import { useDeleteMemory, useMemories } from "./api"

const dateFormat = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" })

/** What the agent chose to remember, with a way to delete wrong or outdated memories. */
export function MemoriesSection({ agent }: { agent: Agent }) {
  const { data: memories, isPending, error } = useMemories(agent.id)
  const remove = useDeleteMemory(agent.id)

  return (
    <section className="flex flex-col gap-2" aria-labelledby="agent-memories">
      <div className="flex items-baseline justify-between">
        <h3 id="agent-memories" className="text-sm font-medium">
          Memories
        </h3>
        {memories && memories.length > 0 ? (
          <span className="text-xs text-muted-foreground tabular-nums">{memories.length}</span>
        ) : null}
      </div>
      {isPending ? (
        <div className="flex flex-col gap-2">
          <Skeleton className="h-9 w-full rounded-lg" />
          <Skeleton className="h-9 w-4/5 rounded-lg" />
        </div>
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load memories: {error.message}</p>
      ) : memories.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nothing saved yet. {agent.name} saves facts and preferences it shouldn't forget, like how
          you like things done.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {memories.map((memory) => (
            <li
              key={memory.id}
              className="group/memory flex items-start gap-2 rounded-lg bg-muted/50 py-2 pr-1 pl-3 text-sm"
            >
              <p className="min-w-0 flex-1 leading-relaxed wrap-break-word">{memory.text}</p>
              <time
                dateTime={memory.createdAt}
                className="shrink-0 pt-0.5 text-xs text-muted-foreground tabular-nums"
              >
                {dateFormat.format(new Date(memory.createdAt))}
              </time>
              <Button
                variant="ghost"
                size="icon-sm"
                className="-my-1 shrink-0 text-muted-foreground opacity-100 hover:text-destructive sm:opacity-0 sm:group-hover/memory:opacity-100 sm:focus-visible:opacity-100"
                aria-label="Delete memory"
                onClick={() =>
                  remove.mutate(memory.id, {
                    onSuccess: () => toast.success("Memory deleted"),
                    onError: (e) => toast.error(`Couldn't delete the memory: ${e.message}`),
                  })
                }
              >
                <Trash2Icon />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

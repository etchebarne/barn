import { BrainIcon, CalendarClockIcon } from "lucide-react"
import { useState } from "react"

import { CommandGroup, CommandItem } from "@/components/ui/command"
import { AgentAvatar, formatRelative } from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"

import { agentAuthorName } from "./preview"
import { splitSnippet, type MessageHit, type SearchResults } from "./search"

function Snippet({ text }: { text: string }) {
  return (
    <span className="line-clamp-2 text-sm wrap-break-word">
      {splitSnippet(text).map((part) =>
        part.match ? (
          <mark key={part.start} className="rounded-sm bg-warning/30 text-foreground">
            {part.text}
          </mark>
        ) : (
          <span key={part.start}>{part.text}</span>
        ),
      )}
    </span>
  )
}

/** The cmdk value of a result: stable while the query changes, so the selection holds. */
export function resultValue(kind: "message" | "memory" | "task", id: string): string {
  return `result ${kind} ${id}`
}

/** The first result, to select when results arrive and nothing else matched. */
export function firstResultValue(results: SearchResults): string | null {
  const message = results.messages.at(0)
  if (message) return resultValue("message", message.id)
  const memory = results.memories.at(0)
  if (memory) return resultValue("memory", memory.id)
  const task = results.tasks.at(0)
  return task ? resultValue("task", task.id) : null
}

function authorName(hit: MessageHit, agents: Map<string, Agent>): string {
  if (hit.author.kind === "user") return "You"
  return agentAuthorName(hit.author.agentId ?? null, agents).name
}

/**
 * The palette's server-side results: messages (with the matched words marked), memories and
 * tasks. They're already filtered by the server, so cmdk mustn't filter them again
 * (`forceMount`).
 */
export function SearchResultGroups({
  results,
  chats,
  agents,
  onMessage,
  onMemory,
  onTask,
}: {
  results: SearchResults
  chats: Map<string, Chat>
  agents: Map<string, Agent>
  onMessage: (hit: MessageHit) => void
  onMemory: (agentId: string) => void
  onTask: (taskId: string) => void
}) {
  // For "3 days ago"; the palette is open for moments, so the time it opened is close enough.
  const [now] = useState(() => new Date())
  return (
    <>
      {results.messages.length > 0 && (
        <CommandGroup heading="Messages" forceMount>
          {results.messages.map((hit) => {
            const chat = chats.get(hit.chatId)
            return (
              <CommandItem
                key={hit.id}
                value={resultValue("message", hit.id)}
                forceMount
                onSelect={() => onMessage(hit)}
                className="items-start"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <Snippet text={hit.snippet} />
                  <span className="truncate text-xs text-muted-foreground">
                    {authorName(hit, agents)} in {chat?.name ?? "a removed chat"} ·{" "}
                    {formatRelative(hit.createdAt, now)}
                  </span>
                </div>
              </CommandItem>
            )
          })}
        </CommandGroup>
      )}
      {results.memories.length > 0 && (
        <CommandGroup heading="Memories" forceMount>
          {results.memories.map((memory) => {
            const agent = agents.get(memory.agentId)
            return (
              <CommandItem
                key={memory.id}
                value={resultValue("memory", memory.id)}
                forceMount
                onSelect={() => onMemory(memory.agentId)}
                className="items-start"
              >
                <BrainIcon className="mt-0.5" />
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="line-clamp-2 text-sm wrap-break-word">{memory.text}</span>
                  <span className="truncate text-xs text-muted-foreground">
                    {agent?.name ?? "Deleted agent"} remembers
                  </span>
                </div>
              </CommandItem>
            )
          })}
        </CommandGroup>
      )}
      {results.tasks.length > 0 && (
        <CommandGroup heading="Tasks" forceMount>
          {results.tasks.map((task) => {
            const agent = agents.get(task.agentId)
            return (
              <CommandItem
                key={task.id}
                value={resultValue("task", task.id)}
                forceMount
                onSelect={() => onTask(task.id)}
              >
                {agent ? (
                  <AgentAvatar id={agent.id} name={agent.name} size="sm" />
                ) : (
                  <CalendarClockIcon />
                )}
                <span className="truncate">{task.name}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {agent?.name ?? "Deleted agent"}
                </span>
              </CommandItem>
            )
          })}
        </CommandGroup>
      )}
    </>
  )
}

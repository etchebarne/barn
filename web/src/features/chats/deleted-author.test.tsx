import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { makeAgent, makeMessage } from "@/test/fixtures"

import { reactorNames } from "./message-reactions"
import { MessageRow } from "./message-row"
import { agentAuthorName, authorName } from "./preview"

const agents = new Map([["agent-1", makeAgent()]])

describe("deleted agents", () => {
  it("are named 'Deleted agent' wherever an author is resolved", () => {
    const message = makeMessage({ author: { kind: "agent", agentId: null } })
    expect(authorName(message, agents)).toBe("Deleted agent")
    expect(agentAuthorName(null, agents)).toEqual({ name: "Deleted agent", deleted: true })
    expect(agentAuthorName("not-loaded", agents)).toEqual({ name: "Agent", deleted: false })
    expect(reactorNames({ emoji: "👍", by: [{ kind: "agent", agentId: null }] }, agents)).toBe(
      "a deleted agent",
    )
  })

  it("render with a neutral avatar and a muted 'Deleted agent' name", () => {
    const message = makeMessage({ author: { kind: "agent", agentId: null }, body: "ship it" })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <TooltipProvider>
          <MessageRow
            row={{ message, startsRun: true, endsRun: true, startsDay: false }}
            agents={agents}
            showAuthorName
            isLatest={false}
          />
        </TooltipProvider>
      </QueryClientProvider>,
    )
    const name = screen.getByText("Deleted agent")
    expect(name).toHaveClass("text-muted-foreground")
    expect(screen.getByText("ship it")).toBeInTheDocument()
    expect(document.body.textContent).not.toContain("undefined")
  })
})

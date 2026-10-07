import { createFileRoute } from "@tanstack/react-router"

import { ComputerPage } from "@/features/computer"

export const Route = createFileRoute("/_app/agents/$agentId/computer")({
  component: ComputerRoute,
})

function ComputerRoute() {
  const { agentId } = Route.useParams()
  // Keyed so another agent's computer starts fresh (tabs, path, shells).
  return <ComputerPage key={agentId} agentId={agentId} />
}

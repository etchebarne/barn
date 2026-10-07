import { createFileRoute, redirect } from "@tanstack/react-router"

import { connectorsForward, readConnectorsSearch } from "@/features/connectors"
import { SettingsPage } from "@/features/settings"

export const Route = createFileRoute("/_app/settings")({
  // Kept only to forward old links and the server's OAuth redirects to /connectors.
  validateSearch: readConnectorsSearch,
  beforeLoad: ({ search }) => {
    const forward = connectorsForward(search)
    if (forward) throw redirect({ to: "/connectors", search: forward, replace: true })
  },
  component: SettingsRoute,
})

function SettingsRoute() {
  const navigate = Route.useNavigate()
  return <SettingsPage onLoggedOut={() => navigate({ to: "/login" })} />
}

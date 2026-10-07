import { createFileRoute } from "@tanstack/react-router"

import { SettingsPage } from "@/features/settings"

export const Route = createFileRoute("/_app/settings")({
  component: SettingsRoute,
})

function SettingsRoute() {
  const navigate = Route.useNavigate()
  return <SettingsPage onLoggedOut={() => navigate({ to: "/login" })} />
}

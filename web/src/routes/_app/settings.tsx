import { createFileRoute } from "@tanstack/react-router"

import { SettingsPage } from "@/features/settings"

type SettingsSearch = {
  /** Open connection in the connectors sheet: "new" (add flow) or a connection id. */
  connector?: string
}

export const Route = createFileRoute("/_app/settings")({
  validateSearch: (search: Record<string, unknown>): SettingsSearch =>
    typeof search.connector === "string" && search.connector ? { connector: search.connector } : {},
  component: SettingsRoute,
})

function SettingsRoute() {
  const navigate = Route.useNavigate()
  const { connector } = Route.useSearch()
  return (
    <SettingsPage
      onLoggedOut={() => navigate({ to: "/login" })}
      connector={connector}
      onConnectorChange={(next) =>
        void navigate({ search: next ? { connector: next } : {}, replace: true })
      }
    />
  )
}

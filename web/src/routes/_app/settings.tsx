import { createFileRoute, redirect } from "@tanstack/react-router"

import { connectorsForward, readConnectorsSearch } from "@/features/connectors"
import { isSettingsPage, SettingsPage } from "@/features/settings"

export const Route = createFileRoute("/_app/settings")({
  // `section` picks the settings page; the connector params are kept only to forward old links
  // and the server's OAuth redirects to /connectors.
  validateSearch: (search: Record<string, unknown>) => ({
    ...readConnectorsSearch(search),
    ...(isSettingsPage(search.section) ? { section: search.section } : {}),
  }),
  beforeLoad: ({ search }) => {
    const { section: _section, ...connectorSearch } = search
    const forward = connectorsForward(connectorSearch)
    if (forward) throw redirect({ to: "/connectors", search: forward, replace: true })
  },
  component: SettingsRoute,
})

function SettingsRoute() {
  const navigate = Route.useNavigate()
  const { section } = Route.useSearch()
  return (
    <SettingsPage
      page={section ?? "general"}
      onPageChange={(page) =>
        void navigate({ search: page === "general" ? {} : { section: page }, replace: true })
      }
      onLoggedOut={() => navigate({ to: "/login" })}
    />
  )
}

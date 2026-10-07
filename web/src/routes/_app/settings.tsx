import { createFileRoute } from "@tanstack/react-router"
import { useCallback, useMemo } from "react"

import { readSignInReturn, useSignInReturn, type SignInReturnParams } from "@/features/connectors"
import { SettingsPage } from "@/features/settings"

type SettingsSearch = SignInReturnParams & {
  /** Open connection in the connectors sheet: "new" (add flow) or a connection id. */
  connector?: string
}

export const Route = createFileRoute("/_app/settings")({
  validateSearch: (search: Record<string, unknown>): SettingsSearch => ({
    ...(typeof search.connector === "string" && search.connector
      ? { connector: search.connector }
      : {}),
    ...readSignInReturn(search),
  }),
  component: SettingsRoute,
})

function SettingsRoute() {
  const navigate = Route.useNavigate()
  const { connector, connected, signin_error } = Route.useSearch()
  const signInReturn = useMemo(
    () => ({ connector, connected, signin_error }),
    [connector, connected, signin_error],
  )
  // Toast the sign-in result, then drop the params (keeping the open connection).
  const clear = useCallback(
    () => void navigate({ search: connector ? { connector } : {}, replace: true }),
    [navigate, connector],
  )
  useSignInReturn(signInReturn, clear)

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

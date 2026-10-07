import { createFileRoute } from "@tanstack/react-router"
import { useCallback, useMemo } from "react"

import { ConnectorsPage, readConnectorsSearch, useSignInReturn } from "@/features/connectors"

export const Route = createFileRoute("/_app/connectors")({
  validateSearch: readConnectorsSearch,
  component: ConnectorsRoute,
})

function ConnectorsRoute() {
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
    <ConnectorsPage
      connector={connector}
      onConnectorChange={(next) =>
        void navigate({ search: next ? { connector: next } : {}, replace: true })
      }
    />
  )
}

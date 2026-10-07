import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef } from "react"
import { toast } from "sonner"

import { queryKeys } from "@/lib/query-keys"

import { useConnectors } from "./api"
import { signInReturnMessage, type SignInReturnParams } from "./signin"

/**
 * After barn's OAuth callback redirects back (`?connected=…` or `?signin_error=…`): toast
 * once, refresh connections, then `clear()` the params from the URL.
 */
export function useSignInReturn(params: SignInReturnParams, clear: () => void) {
  const queryClient = useQueryClient()
  const connectors = useConnectors()
  const handled = useRef(false)
  const active = !!params.connected || !!params.signin_error
  // Wait for the connections list so the toast can name the new one.
  const ready = !params.connected || !connectors.isPending

  useEffect(() => {
    if (!active || !ready || handled.current) return
    handled.current = true
    const message = signInReturnMessage(params, connectors.data)
    if (message?.kind === "success") toast.success(message.text)
    else if (message) toast.error(message.text)
    if (params.connected) void queryClient.invalidateQueries({ queryKey: queryKeys.connectors })
    clear()
  }, [active, ready, params, connectors.data, queryClient, clear])
}

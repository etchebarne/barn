import { useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import { applyWsEvent, RealtimeClient, resyncAfterReconnect } from "@/lib/ws"

/** Keeps the single WebSocket open while the signed-in app is mounted. */
export function useRealtime() {
  const queryClient = useQueryClient()
  useEffect(() => {
    const client = new RealtimeClient({
      onEvent: (event) => applyWsEvent(queryClient, event),
      onReconnect: () => void resyncAfterReconnect(queryClient),
    })
    client.start()
    return () => client.stop()
  }, [queryClient])
}

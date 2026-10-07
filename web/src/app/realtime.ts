import { useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import { useLeaveRemovedChats } from "@/lib/leave-removed-chats"
import { applyWsEvent, RealtimeClient, resyncAfterReconnect } from "@/lib/ws"

/** Keeps the single WebSocket open while the signed-in app is mounted. */
export function useRealtime() {
  const queryClient = useQueryClient()
  const leaveRemovedChats = useLeaveRemovedChats()
  useEffect(() => {
    const client = new RealtimeClient({
      onEvent: (event) => {
        const { removedChatIds } = applyWsEvent(queryClient, event)
        if (removedChatIds.length > 0) leaveRemovedChats(removedChatIds)
      },
      onReconnect: () => void resyncAfterReconnect(queryClient),
    })
    client.start()
    return () => client.stop()
  }, [queryClient, leaveRemovedChats])
}

import { useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"

import { useLeaveRemovedChats } from "@/lib/leave-removed-chats"
import { applyWsEvent, RealtimeClient, resyncAfterReconnect } from "@/lib/ws"

import { notifyDesktop } from "./desktop"

/** Keeps the single WebSocket open while the signed-in app is mounted. */
export function useRealtime() {
  const queryClient = useQueryClient()
  const leaveRemovedChats = useLeaveRemovedChats()
  useEffect(() => {
    const client = new RealtimeClient({
      onEvent: (event) => {
        // Before applying it, so the chat list still says whether this message is new.
        if (event.type === "message.created") notifyDesktop(queryClient, event.message)
        const { removedChatIds } = applyWsEvent(queryClient, event)
        if (removedChatIds.length > 0) leaveRemovedChats(removedChatIds)
      },
      onReconnect: () => void resyncAfterReconnect(queryClient),
    })
    client.start()
    return () => client.stop()
  }, [queryClient, leaveRemovedChats])
}

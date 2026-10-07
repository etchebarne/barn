import { useRouter } from "@tanstack/react-router"
import { useCallback } from "react"

import { chatPath } from "./chat-cache"

/**
 * Returns a function that, if the user is viewing one of `removedChatIds`, sends them to the
 * first remaining chat (the index route redirects there).
 */
export function useLeaveRemovedChats() {
  const router = useRouter()
  return useCallback(
    (removedChatIds: string[]) => {
      const path = router.state.location.pathname
      if (removedChatIds.some((id) => path === chatPath(id))) {
        void router.navigate({ to: "/", replace: true })
      }
    },
    [router],
  )
}

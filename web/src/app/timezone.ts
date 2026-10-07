import { useEffect } from "react"

import { api } from "@/lib/api-client"

let sent = false

/** Tells the server the browser's time zone once per session (fire and forget). */
export function useSyncTimezone() {
  useEffect(() => {
    if (sent) return
    sent = true
    let timezone: string
    try {
      timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
    } catch {
      return
    }
    if (!timezone) return
    api.PUT("/settings/timezone", { body: { timezone } }).catch(() => {
      // Ignored: schedules fall back to the server's zone.
    })
  }, [])
}

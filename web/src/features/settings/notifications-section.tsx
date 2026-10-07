import { useQuery, useQueryClient } from "@tanstack/react-query"
import { BellIcon, BellOffIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { disablePush, enablePush, readPushState, type PushState } from "@/lib/push"

import { SettingsSection } from "./settings-section"

const PUSH_STATE_KEY = ["push", "state"] as const

const DESCRIPTIONS: Record<PushState, string> = {
  unsupported:
    typeof window !== "undefined" && !window.isSecureContext
      ? "Notifications need openbot to be opened over HTTPS (for example with Tailscale Serve)."
      : "This browser can't show notifications from openbot.",
  unavailable: "Notifications aren't set up on the server.",
  blocked:
    "Notifications are blocked for openbot. Allow them in your browser's site settings, then reload.",
  off: "Off for this device.",
  on: "On for this device. Agents with notifications enabled will reach you here.",
}

/** Push notifications for this device: shows the current state and turns them on or off. */
export function NotificationsSection() {
  const queryClient = useQueryClient()
  const state = useQuery({ queryKey: PUSH_STATE_KEY, queryFn: readPushState, staleTime: 0 })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function change(action: () => Promise<PushState>) {
    setBusy(true)
    setError(null)
    try {
      const next = await action()
      queryClient.setQueryData(PUSH_STATE_KEY, next)
      if (next === "on") toast.success("Notifications are on for this device")
      else if (next === "off" && state.data === "on") toast.success("Notifications are off")
    } catch (e) {
      setError(e instanceof Error ? e.message : "Something went wrong.")
    } finally {
      setBusy(false)
    }
  }

  const current = state.data
  return (
    <SettingsSection
      id="notifications"
      title="Notifications"
      description="Get a notification when an agent messages you and openbot isn't open."
    >
      {state.isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : state.error ? (
        <p className="text-sm text-destructive">
          Couldn't check notifications: {state.error.message}
        </p>
      ) : (
        <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
          <div className="flex min-w-0 items-start gap-3">
            {current === "on" ? (
              <BellIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            ) : (
              <BellOffIcon
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            )}
            <p
              className={current === "blocked" ? "text-sm text-warning-foreground" : "text-sm"}
              role="status"
            >
              {current && DESCRIPTIONS[current]}
            </p>
          </div>
          {current === "off" && (
            <Button size="sm" disabled={busy} onClick={() => void change(enablePush)}>
              {busy && <Spinner />}
              Enable
            </Button>
          )}
          {current === "on" && (
            <Button
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={() => void change(disablePush)}
            >
              {busy && <Spinner />}
              Disable
            </Button>
          )}
        </div>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        On iPhone and iPad, add openbot to your Home Screen (Share → Add to Home Screen) and open it
        from there to get notifications.
      </p>
    </SettingsSection>
  )
}

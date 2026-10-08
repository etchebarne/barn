import { DownloadIcon, ExternalLinkIcon, MonitorIcon } from "lucide-react"
import { useState } from "react"

import { Button, buttonVariants } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import {
  desktop,
  DESKTOP_DOWNLOAD_URL,
  desktopNotificationsEnabled,
  setDesktopNotificationsEnabled,
} from "@/lib/desktop"

import { SettingsSection } from "./settings-section"

/** In the desktop app: native notifications from the live connection, on or off per device. */
export function DesktopNotificationsSection() {
  const [on, setOn] = useState(desktopNotificationsEnabled)
  return (
    <SettingsSection
      id="notifications"
      title="Notifications"
      description="Get a notification when an agent messages you and you're not looking at that chat."
    >
      <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <label htmlFor="desktop-notifications" className="text-sm font-medium">
            Desktop notifications
          </label>
          <p className="text-sm text-muted-foreground">
            For this computer. Each agent's own Notifications switch still applies.
          </p>
        </div>
        <Switch
          id="desktop-notifications"
          aria-label="Desktop notifications"
          checked={on}
          onCheckedChange={(checked: boolean) => {
            setDesktopNotificationsEnabled(checked)
            setOn(checked)
          }}
        />
      </div>
    </SettingsSection>
  )
}

/** Which server the desktop app is connected to, or where to get the app. */
export function DesktopAppSection() {
  const bridge = desktop()
  return (
    <SettingsSection
      id="desktop-app"
      title="Desktop app"
      description={
        bridge ? undefined : "openbot in its own window, with notifications and an unread badge."
      }
    >
      {bridge ? (
        <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
          <div className="flex min-w-0 items-center gap-3">
            <MonitorIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <p className="min-w-0 truncate text-sm">
              Connected to <span className="font-medium">{window.location.origin}</span>
            </p>
          </div>
          <Button size="sm" variant="outline" onClick={() => bridge.changeServer()}>
            Change server…
          </Button>
        </div>
      ) : (
        <a
          href={DESKTOP_DOWNLOAD_URL}
          target="_blank"
          rel="noreferrer"
          className={buttonVariants({ variant: "outline", size: "sm", className: "w-fit" })}
        >
          <DownloadIcon aria-hidden="true" />
          Get the desktop app
          <ExternalLinkIcon aria-hidden="true" className="text-muted-foreground" />
        </a>
      )}
    </SettingsSection>
  )
}

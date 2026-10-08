import { PageHeader } from "@/components/page-header"
import { isDesktop } from "@/lib/desktop"

import { AccountSection } from "./account-section"
import { AppearanceSection } from "./appearance-section"
import { BackupSection } from "./backup-section"
import { DesktopAppSection, DesktopNotificationsSection } from "./desktop-sections"
import { NotificationsSection } from "./notifications-section"
import { ProviderSection } from "./provider-section"
import { UsageSection } from "./usage-section"

export function SettingsPage({ onLoggedOut }: { onLoggedOut: () => Promise<void> }) {
  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="truncate text-sm font-medium">Settings</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-4 py-2 md:px-6">
          <AppearanceSection />
          {/* Web Push doesn't work in the desktop app; it notifies from the live connection. */}
          {isDesktop() ? <DesktopNotificationsSection /> : <NotificationsSection />}
          <DesktopAppSection />
          <ProviderSection />
          <UsageSection />
          <BackupSection />
          <AccountSection onLoggedOut={onLoggedOut} />
        </div>
      </div>
    </div>
  )
}

import { PageHeader } from "@/components/page-header"
import { ConnectorsSection } from "@/features/connectors"

import { AccountSection } from "./account-section"
import { AppearanceSection } from "./appearance-section"
import { NotificationsSection } from "./notifications-section"
import { ProviderSection } from "./provider-section"

export function SettingsPage({
  onLoggedOut,
  connector,
  onConnectorChange,
}: {
  onLoggedOut: () => Promise<void>
  /** The connection shown in the connectors sheet ("new" for the add flow). */
  connector: string | undefined
  onConnectorChange: (connector: string | undefined) => void
}) {
  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="truncate text-sm font-medium">Settings</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-4 py-2 md:px-6">
          <AppearanceSection />
          <NotificationsSection />
          <ProviderSection />
          <ConnectorsSection selection={connector} onSelect={onConnectorChange} />
          <AccountSection onLoggedOut={onLoggedOut} />
        </div>
      </div>
    </div>
  )
}

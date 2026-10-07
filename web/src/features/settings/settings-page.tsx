import { PageHeader } from "@/components/page-header"

import { AccountSection } from "./account-section"
import { AppearanceSection } from "./appearance-section"
import { ProviderSection } from "./provider-section"

export function SettingsPage({ onLoggedOut }: { onLoggedOut: () => Promise<void> }) {
  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="truncate text-sm font-medium">Settings</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-4 py-2 md:px-6">
          <AppearanceSection />
          <ProviderSection />
          <AccountSection onLoggedOut={onLoggedOut} />
        </div>
      </div>
    </div>
  )
}

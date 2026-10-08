import { cn } from "cn"
import { CircleUserIcon, CpuIcon, SlidersHorizontalIcon, type LucideIcon } from "lucide-react"
import type { ReactNode } from "react"

import { PageHeader } from "@/components/page-header"
import { SegmentedControl } from "@/components/segmented-control"
import { useIsMobile } from "@/hooks/use-mobile"
import { isDesktop } from "@/lib/desktop"

import { AccountSection } from "./account-section"
import { AppearanceSection } from "./appearance-section"
import { BackupSection } from "./backup-section"
import { DesktopAppSection, DesktopNotificationsSection } from "./desktop-sections"
import { NotificationsSection } from "./notifications-section"
import { ProviderSection } from "./provider-section"
import { UsageSection } from "./usage-section"

export const SETTINGS_PAGES = ["general", "models", "account"] as const
export type SettingsPageId = (typeof SETTINGS_PAGES)[number]

export function isSettingsPage(value: unknown): value is SettingsPageId {
  return typeof value === "string" && (SETTINGS_PAGES as readonly string[]).includes(value)
}

const PAGES: Record<SettingsPageId, { label: string; description: string; icon: LucideIcon }> = {
  general: {
    label: "General",
    description: "How openbot looks and how it reaches you on this device.",
    icon: SlidersHorizontalIcon,
  },
  models: {
    label: "Models & usage",
    description: "The provider your agents run on, and what they've used.",
    icon: CpuIcon,
  },
  account: {
    label: "Account & data",
    description: "Your sign-in, and backups of your agents' setup.",
    icon: CircleUserIcon,
  },
}

function PageBody({
  page,
  onLoggedOut,
}: {
  page: SettingsPageId
  onLoggedOut: () => Promise<void>
}) {
  switch (page) {
    case "general":
      return (
        <>
          <AppearanceSection />
          {/* Web Push doesn't work in the desktop app; it notifies from the live connection. */}
          {isDesktop() ? <DesktopNotificationsSection /> : <NotificationsSection />}
          <DesktopAppSection />
        </>
      )
    case "models":
      return (
        <>
          <ProviderSection />
          <UsageSection />
        </>
      )
    case "account":
      return (
        <>
          <BackupSection />
          <AccountSection onLoggedOut={onLoggedOut} />
        </>
      )
    default:
      return null
  }
}

function NavItem({
  page,
  current,
  onSelect,
}: {
  page: SettingsPageId
  current: boolean
  onSelect: () => void
}) {
  const { label, icon: Icon } = PAGES[page]
  return (
    <button
      type="button"
      aria-current={current ? "page" : undefined}
      className={cn(
        "flex h-8 items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50 [&_svg]:size-4",
        current
          ? "bg-accent font-medium text-foreground"
          : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
      )}
      onClick={onSelect}
    >
      <Icon aria-hidden="true" />
      {label}
    </button>
  )
}

/**
 * Settings, split into a few pages (General, Models & usage, Account) picked from a list on the
 * left (a segmented control on phones). The page lives in the URL (`?section=`).
 */
const ignorePageChange = () => {}

export function SettingsPage({
  page = "general",
  onPageChange = ignorePageChange,
  onLoggedOut,
}: {
  page?: SettingsPageId
  onPageChange?: (page: SettingsPageId) => void
  onLoggedOut: () => Promise<void>
}) {
  const mobile = useIsMobile()
  const { label, description } = PAGES[page]
  const body: ReactNode = <PageBody page={page} onLoggedOut={onLoggedOut} />
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
      {mobile && (
        <PageHeader>
          <h1 className="truncate text-sm font-semibold">Settings</h1>
        </PageHeader>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-4xl gap-10 px-4 pt-4 pb-16 md:px-8 md:pt-10">
          {!mobile && (
            <nav
              aria-label="Settings"
              className="sticky top-0 flex h-fit w-48 shrink-0 flex-col gap-0.5"
            >
              <h1 className="mb-4 px-2.5 text-2xl font-semibold tracking-tight">Settings</h1>
              {SETTINGS_PAGES.map((p) => (
                <NavItem key={p} page={p} current={p === page} onSelect={() => onPageChange(p)} />
              ))}
            </nav>
          )}
          <div className="flex min-w-0 flex-1 flex-col">
            {mobile ? (
              <SegmentedControl
                label="Settings"
                value={page}
                onChange={onPageChange}
                className="mb-2 self-start"
                options={SETTINGS_PAGES.map((p) => ({ value: p, label: PAGES[p].label }))}
              />
            ) : (
              <header className="flex flex-col gap-1 border-b pt-1.5 pb-5">
                <h2 className="text-lg font-semibold tracking-tight">{label}</h2>
                <p className="text-sm text-muted-foreground">{description}</p>
              </header>
            )}
            {body}
          </div>
        </div>
      </div>
    </div>
  )
}

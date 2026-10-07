import { useQuery } from "@tanstack/react-query"
import { LogOutIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { authStatusQueryOptions, useLogout } from "@/features/auth"

import { SettingsSection } from "./settings-section"

export function AccountSection({ onLoggedOut }: { onLoggedOut: () => Promise<void> }) {
  const { data } = useQuery(authStatusQueryOptions)
  const logout = useLogout(onLoggedOut)

  return (
    <SettingsSection title="Account">
      <div className="flex items-center justify-between gap-3">
        <p className="min-w-0 truncate text-sm">
          Signed in as <span className="font-medium">{data?.user?.username ?? "…"}</span>
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={logout.isPending}
          onClick={() => logout.mutate()}
        >
          {logout.isPending ? <Spinner /> : <LogOutIcon />}
          Log out
        </Button>
      </div>
    </SettingsSection>
  )
}

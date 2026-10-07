import { useQuery } from "@tanstack/react-query"
import { KeyRoundIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import { formatKeyHint, providerSettingsQueryOptions } from "./provider-api"
import { ProviderKeyForm } from "./provider-key-form"
import { SettingsSection } from "./settings-section"

export function ProviderSection() {
  const { data, isPending, error } = useQuery(providerSettingsQueryOptions)
  const [replacing, setReplacing] = useState(false)
  const hint = formatKeyHint(data?.keyHint)

  return (
    <SettingsSection
      title="Model provider"
      description="Agents use your OpenCode Go subscription for every model call."
    >
      {isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : error ? (
        <p className="text-sm text-destructive">Couldn't load provider settings: {error.message}</p>
      ) : replacing ? (
        <ProviderKeyForm
          autoFocus
          submitLabel="Save key"
          onSaved={() => setReplacing(false)}
          secondaryAction={
            <Button type="button" variant="ghost" onClick={() => setReplacing(false)}>
              Cancel
            </Button>
          }
        />
      ) : (
        <div className="flex items-center justify-between gap-3 rounded-lg border p-3">
          <div className="flex min-w-0 items-center gap-3">
            <KeyRoundIcon className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate text-sm">
              OpenCode Go
              <span className="text-muted-foreground">
                {data.configured && hint ? ` · key ${hint}` : " · not configured"}
              </span>
            </span>
          </div>
          <Button variant="outline" size="sm" onClick={() => setReplacing(true)}>
            {data.configured ? "Replace key" : "Add key"}
          </Button>
        </div>
      )}
    </SettingsSection>
  )
}

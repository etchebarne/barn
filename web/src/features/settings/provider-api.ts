import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type OnboardingState, type ProviderSettings } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export const providerSettingsQueryOptions = queryOptions({
  queryKey: queryKeys.providerSettings,
  queryFn: () => unwrap(api.GET("/settings/provider")),
})

/** Validates the key against OpenCode Go and saves it. A 400 means the key was rejected. */
export function useUpdateProviderKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (apiKey: string) => unwrap(api.PUT("/settings/provider", { body: { apiKey } })),
    onSuccess: (settings) => {
      queryClient.setQueryData<ProviderSettings>(queryKeys.providerSettings, settings)
      queryClient.setQueryData<OnboardingState>(queryKeys.onboarding, (state) =>
        state ? { ...state, providerConfigured: settings.configured } : state,
      )
      void queryClient.invalidateQueries({ queryKey: queryKeys.models })
    },
  })
}

/** "…a1b2" regardless of whether the server already includes the ellipsis. */
export function formatKeyHint(hint: string | null | undefined): string | null {
  if (!hint) return null
  return `…${hint.replace(/^(…|\.\.\.)/, "")}`
}

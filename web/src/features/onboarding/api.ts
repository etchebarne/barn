import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type OnboardingState } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export const onboardingQueryOptions = queryOptions({
  queryKey: queryKeys.onboarding,
  queryFn: () => unwrap(api.GET("/onboarding")),
  staleTime: Number.POSITIVE_INFINITY,
})

export const DEFAULT_AGENT_NAME = "openbot"

export function useCompleteOnboarding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: { model: string; agentName?: string }) =>
      unwrap(api.POST("/onboarding/complete", { body })),
    onSuccess: async () => {
      queryClient.setQueryData<OnboardingState>(queryKeys.onboarding, {
        providerConfigured: true,
        starterAgentCreated: true,
        completed: true,
      })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.chats }),
        queryClient.invalidateQueries({ queryKey: queryKeys.agents }),
      ])
    },
  })
}

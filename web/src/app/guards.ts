import type { QueryClient } from "@tanstack/react-query"
import { redirect } from "@tanstack/react-router"

import { authStatusQueryOptions } from "@/features/auth"
import { onboardingQueryOptions } from "@/features/onboarding"

/** Redirects to /login unless signed in. */
export async function requireUser(queryClient: QueryClient) {
  const status = await queryClient.ensureQueryData(authStatusQueryOptions)
  if (!status.user) throw redirect({ to: "/login" })
  return status.user
}

/** Redirects to /onboarding until the provider and starter agent are set up. */
export async function requireOnboarded(queryClient: QueryClient) {
  await requireUser(queryClient)
  const onboarding = await queryClient.ensureQueryData(onboardingQueryOptions)
  if (!onboarding.completed) throw redirect({ to: "/onboarding" })
}

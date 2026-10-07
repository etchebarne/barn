import { createFileRoute, redirect } from "@tanstack/react-router"

import { requireUser } from "@/app/guards"
import { onboardingQueryOptions, OnboardingScreen } from "@/features/onboarding"

export const Route = createFileRoute("/onboarding")({
  beforeLoad: async ({ context }) => {
    await requireUser(context.queryClient)
    const state = await context.queryClient.ensureQueryData(onboardingQueryOptions)
    if (state.completed) throw redirect({ to: "/" })
  },
  component: OnboardingRoute,
})

function OnboardingRoute() {
  const navigate = Route.useNavigate()
  return (
    <OnboardingScreen
      onComplete={(chatId) => void navigate({ to: "/chats/$chatId", params: { chatId } })}
    />
  )
}

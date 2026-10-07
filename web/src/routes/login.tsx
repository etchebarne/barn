import { createFileRoute, redirect } from "@tanstack/react-router"

import { authStatusQueryOptions, AuthScreen } from "@/features/auth"

export const Route = createFileRoute("/login")({
  beforeLoad: async ({ context }) => {
    const status = await context.queryClient.ensureQueryData(authStatusQueryOptions)
    if (status.user) throw redirect({ to: "/" })
  },
  component: LoginRoute,
})

function LoginRoute() {
  const navigate = Route.useNavigate()
  return <AuthScreen onAuthenticated={() => void navigate({ to: "/" })} />
}

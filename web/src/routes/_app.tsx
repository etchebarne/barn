import { createFileRoute } from "@tanstack/react-router"

import { AppShell } from "@/app/app-shell"
import { requireOnboarded } from "@/app/guards"
import { agentsQueryOptions } from "@/features/agents"
import { chatsQueryOptions } from "@/features/chats"

export const Route = createFileRoute("/_app")({
  beforeLoad: ({ context }) => requireOnboarded(context.queryClient),
  loader: ({ context }) => {
    // Warm the sidebar data; don't block navigation on it.
    void context.queryClient.prefetchQuery(chatsQueryOptions)
    void context.queryClient.prefetchQuery(agentsQueryOptions)
  },
  component: AppShell,
})

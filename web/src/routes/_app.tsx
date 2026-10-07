import { createFileRoute } from "@tanstack/react-router"

import { AppShell } from "@/app/app-shell"
import { requireOnboarded } from "@/app/guards"
import { chatsQueryOptions } from "@/features/chats"
import { agentsQueryOptions } from "@/lib/agents"

export const Route = createFileRoute("/_app")({
  beforeLoad: ({ context }) => requireOnboarded(context.queryClient),
  loader: ({ context }) => {
    // Warm the sidebar data; don't block navigation on it.
    void context.queryClient.prefetchQuery(chatsQueryOptions)
    void context.queryClient.prefetchQuery(agentsQueryOptions)
  },
  component: AppShell,
})

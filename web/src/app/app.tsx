import { QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider } from "@tanstack/react-router"
import { useEffect, useState } from "react"

import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import type { AuthStatus } from "@/lib/api-client"
import { setUnauthorizedHandler } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { watchSystemTheme } from "@/lib/theme"

import { createQueryClient } from "./query-client"
import { createAppRouter } from "./router"

export function App() {
  const [queryClient] = useState(createQueryClient)
  const [router] = useState(() => createAppRouter(queryClient))

  useEffect(() => watchSystemTheme(), [])

  useEffect(() => {
    // Session expired or revoked: drop cached data and go to the login screen.
    setUnauthorizedHandler(() => {
      queryClient.clear()
      queryClient.setQueryData<AuthStatus>(queryKeys.authStatus, {
        setupRequired: false,
        user: null,
      })
      void router.navigate({ to: "/login" })
    })
    return () => setUnauthorizedHandler(null)
  }, [queryClient, router])

  return (
    <QueryClientProvider client={queryClient}>
      {/* One provider so adjacent tooltips open instantly after the first (grouped). */}
      <TooltipProvider delay={500} closeDelay={0} timeout={400}>
        <RouterProvider router={router} />
        <Toaster position="top-center" />
      </TooltipProvider>
    </QueryClientProvider>
  )
}

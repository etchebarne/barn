import {
  createRootRouteWithContext,
  Outlet,
  type ErrorComponentProps,
} from "@tanstack/react-router"

import type { RouterContext } from "@/app/router"
import { Button } from "@/components/ui/button"

function RootError({ error, reset }: ErrorComponentProps) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-3 p-6 text-center">
      <h1 className="text-sm font-medium">Can't reach openbot</h1>
      <p className="max-w-sm text-sm text-muted-foreground">
        {error instanceof Error ? error.message : "Something went wrong."} Check that the server is
        running, then try again.
      </p>
      <Button variant="outline" size="sm" onClick={reset}>
        Try again
      </Button>
    </div>
  )
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  errorComponent: RootError,
  notFoundComponent: () => (
    <div className="flex min-h-svh items-center justify-center p-6 text-sm text-muted-foreground">
      Page not found.
    </div>
  ),
})

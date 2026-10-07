import createClient, { type Middleware } from "openapi-fetch"

import type { components, paths } from "./api/schema.gen"

export type Schemas = components["schemas"]
export type AuthStatus = Schemas["AuthStatus"]
export type User = Schemas["User"]
export type Agent = Schemas["Agent"]
export type AgentActivity = Schemas["AgentActivity"]
export type Chat = Schemas["Chat"]
export type Message = Schemas["Message"]
export type MessagePage = Schemas["MessagePage"]
export type Model = Schemas["Model"]
export type OnboardingState = Schemas["OnboardingState"]
export type ProviderSettings = Schemas["ProviderSettings"]
export type WsEvent = Schemas["WsEvent"]

/** Header the server requires on every request (checked together with Origin). */
export const CSRF_HEADER = "X-Barn-CSRF"

/** Error carrying the server's `{ message }` body and HTTP status. */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

let unauthorizedHandler: (() => void) | null = null

/**
 * Called on any 401 outside of the auth endpoints (the session expired or was revoked).
 * The app registers a handler that resets auth state and routes to the login screen.
 */
export function setUnauthorizedHandler(handler: (() => void) | null) {
  unauthorizedHandler = handler
}

const authMiddleware: Middleware = {
  onRequest({ request }) {
    request.headers.set(CSRF_HEADER, "1")
    return request
  },
  onResponse({ request, response }) {
    if (response.status === 401 && !new URL(request.url).pathname.includes("/api/auth/")) {
      unauthorizedHandler?.()
    }
    return response
  },
}

export const api = createClient<paths>({
  baseUrl: "/api",
  credentials: "include",
})
api.use(authMiddleware)

function errorMessage(error: unknown, fallback: string): string {
  if (
    error &&
    typeof error === "object" &&
    "message" in error &&
    typeof error.message === "string"
  ) {
    return error.message
  }
  return fallback
}

/**
 * Unwraps an openapi-fetch result: returns `data` or throws an `ApiError` with the
 * server's message, so TanStack Query sees failures as errors.
 */
export async function unwrap<T>(
  promise: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  const { data, error, response } = await promise
  if (!response.ok) {
    throw new ApiError(response.status, errorMessage(error, `Request failed (${response.status})`))
  }
  // 204 responses have no body; callers expecting data only use endpoints that return it.
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion
  return data as T
}

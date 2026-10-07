import { queryOptions } from "@tanstack/react-query"

import { api, ApiError, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

import type { Connector } from "./logic"

export type CatalogApp = Schemas["CatalogApp"]
export type StartSignInRequest = Schemas["StartSignInRequest"]
export type StartSignInResponse = Schemas["StartSignInResponse"]
export type SignInResult = Schemas["SignInResult"]

export const catalogQueryOptions = queryOptions({
  queryKey: queryKeys.connectorCatalog,
  queryFn: () => unwrap(api.GET("/connectors/catalog")),
  staleTime: 10 * 60_000,
})

/** The server calls the sign-in flow; injectable so the UI can be tested without a network. */
export type SignInDeps = {
  start: (body: StartSignInRequest) => Promise<StartSignInResponse>
  complete: (callbackUrl: string) => Promise<SignInResult>
  navigate: { assign: (url: string) => void; open: (url: string) => void }
}

export const defaultSignInDeps: SignInDeps = {
  start: (body) => unwrap(api.POST("/connectors/sign-in", { body })),
  complete: (callbackUrl) =>
    unwrap(api.POST("/connectors/sign-in/complete", { body: { callbackUrl } })),
  navigate: {
    assign: (url) => window.location.assign(url),
    open: (url) => {
      window.open(url, "_blank", "noopener,noreferrer")
    },
  },
}

/**
 * Sends the browser to the app's sign-in page. Normally in this tab (the app redirects back
 * to barn). When the app won't redirect to barn's address ("paste back"), the page opens in a
 * new tab and the user pastes the address they land on. Returns which happened.
 */
export function launchSignIn(
  response: StartSignInResponse,
  navigate: SignInDeps["navigate"],
): "redirect" | "paste" {
  if (response.pasteBack) {
    navigate.open(response.authorizeUrl)
    return "paste"
  }
  navigate.assign(response.authorizeUrl)
  return "redirect"
}

/** Query params barn's OAuth callback redirects with. */
export type SignInReturnParams = { connected?: string; signin_error?: string; connector?: string }

/** Reads the sign-in return params from a route's raw search. */
export function readSignInReturn(search: Record<string, unknown>): SignInReturnParams {
  const pick = (key: string) => {
    const value = search[key]
    if (typeof value === "string" && value) return value
    if (typeof value === "number") return String(value)
    return undefined
  }
  const params: SignInReturnParams = {}
  const connected = pick("connected")
  const error = pick("signin_error")
  if (connected) params.connected = connected
  if (error) params.signin_error = error
  return params
}

export type SignInReturn = { kind: "success" | "error"; text: string } | null

/**
 * The toast for a return from sign-in (`?connected=<id>` or `?signin_error=<message>`).
 * The connection's name is used once it's known.
 */
export function signInReturnMessage(
  params: SignInReturnParams,
  connectors: Connector[] | undefined,
): SignInReturn {
  if (params.signin_error) return { kind: "error", text: params.signin_error }
  if (params.connected) {
    // Settings: `?connector=<id>&connected=1`. Chats: `?connected=<accountId>`.
    const id = params.connected === "1" ? params.connector : params.connected
    const name = connectors?.find((c) => c.id === id)?.name
    return { kind: "success", text: name ? `Connected ${name}` : "Connected" }
  }
  return null
}

/** Which form a connection create error should switch to. */
export function needsSignIn(error: unknown): boolean {
  return error instanceof ApiError && error.code === "sign_in_required"
}

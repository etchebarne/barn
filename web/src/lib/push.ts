import { api, ApiError, unwrap } from "./api-client"

/**
 * Notification state for this device:
 * - unsupported: no service worker / Push API (e.g. iPhone Safari outside the Home Screen app)
 * - unavailable: the server has no push key configured
 * - blocked: the user denied notifications for this site
 * - off / on: whether this browser has a push subscription
 */
export type PushState = "unsupported" | "unavailable" | "blocked" | "off" | "on"

export function pushState(input: {
  supported: boolean
  serverAvailable: boolean
  permission: NotificationPermission
  subscribed: boolean
}): PushState {
  if (!input.supported) return "unsupported"
  if (!input.serverAvailable) return "unavailable"
  if (input.permission === "denied") return "blocked"
  return input.subscribed && input.permission === "granted" ? "on" : "off"
}

export function isPushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  )
}

/** VAPID keys are base64url; the Push API wants bytes. */
export function urlBase64ToUint8Array(base64url: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64url.length % 4)) % 4)
  const base64 = (base64url + padding).replace(/-/g, "+").replace(/_/g, "/")
  const raw = atob(base64)
  const bytes = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
  return bytes
}

const SW_URL = "/sw.js"

/** Registers the service worker that shows notifications. Safe to call more than once. */
export function registerServiceWorker() {
  if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return
  navigator.serviceWorker.register(SW_URL).catch(() => {
    // Notifications just won't work; the app itself is unaffected.
  })
}

/** The active registration, registering (and waiting for) the worker if needed. */
async function activeRegistration(): Promise<ServiceWorkerRegistration> {
  await navigator.serviceWorker.register(SW_URL)
  return navigator.serviceWorker.ready
}

/** The server's VAPID key, or null when push isn't configured (503). */
export async function fetchPushKey(): Promise<string | null> {
  try {
    const { publicKey } = await unwrap(api.GET("/push/config"))
    return publicKey
  } catch (error) {
    if (error instanceof ApiError && error.status === 503) return null
    throw error
  }
}

/** This browser's push subscription, without waiting on a worker that may never activate. */
async function currentSubscription(): Promise<PushSubscription | null> {
  const registration = await navigator.serviceWorker.getRegistration()
  return (await registration?.pushManager.getSubscription()) ?? null
}

export async function readPushState(): Promise<PushState> {
  if (!isPushSupported()) return "unsupported"
  const key = await fetchPushKey()
  return pushState({
    supported: true,
    serverAvailable: key !== null,
    permission: Notification.permission,
    subscribed: (await currentSubscription()) !== null,
  })
}

/** Asks for permission, subscribes this browser and registers it with the server. */
export async function enablePush(): Promise<PushState> {
  const key = await fetchPushKey()
  if (!key) return "unavailable"
  const permission = await Notification.requestPermission()
  if (permission !== "granted") return permission === "denied" ? "blocked" : "off"
  const registration = await activeRegistration()
  const subscription =
    (await registration.pushManager.getSubscription()) ??
    (await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(key),
    }))
  const json = subscription.toJSON()
  await unwrap(
    api.POST("/push/subscriptions", {
      body: {
        endpoint: subscription.endpoint,
        keys: { p256dh: json.keys?.p256dh ?? "", auth: json.keys?.auth ?? "" },
      },
    }),
  )
  return "on"
}

/** Unsubscribes this browser and tells the server to forget it. */
export async function disablePush(): Promise<PushState> {
  const subscription = await currentSubscription()
  if (subscription) {
    const { endpoint } = subscription
    await subscription.unsubscribe()
    await unwrap(api.DELETE("/push/subscriptions", { body: { endpoint } }))
  }
  return "off"
}

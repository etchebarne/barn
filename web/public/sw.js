/* openbot service worker: shows push notifications and focuses the app when one is clicked. */

self.addEventListener("install", () => self.skipWaiting())
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()))

function chatUrl(chatId) {
  return chatId ? `/chats/${encodeURIComponent(chatId)}` : "/"
}

self.addEventListener("push", (event) => {
  let payload = {}
  try {
    payload = event.data ? event.data.json() : {}
  } catch {
    payload = { body: event.data ? event.data.text() : "" }
  }
  const url = chatUrl(payload.chatId)

  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      // Already reading that chat: no need to notify.
      const reading = clients.some(
        (client) =>
          client.focused &&
          client.visibilityState === "visible" &&
          new URL(client.url).pathname === url,
      )
      if (reading) return undefined
      return self.registration.showNotification(payload.title || "openbot", {
        body: payload.body || "",
        tag: payload.tag,
        data: { url },
        icon: "/icon-192.png",
        badge: "/badge-96.png",
      })
    }),
  )
})

self.addEventListener("notificationclick", (event) => {
  event.notification.close()
  const url = (event.notification.data && event.notification.data.url) || "/"
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(async (clients) => {
      const client = clients.find((c) => new URL(c.url).origin === self.location.origin)
      if (client) {
        await client.focus()
        if ("navigate" in client) return client.navigate(url)
        return undefined
      }
      return self.clients.openWindow(url)
    }),
  )
})

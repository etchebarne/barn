// The bridge between the openbot web app (or the local connect page) and the desktop shell.
// Kept tiny: the main process checks which page each call comes from.
import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron"

contextBridge.exposeInMainWorld("openbotDesktop", {
  platform: process.platform,
  /** Shows a native notification; clicking it opens chatId. */
  notify: (n: { title: string; body: string; chatId?: string; tag?: string }) =>
    ipcRenderer.send("openbot:notify", n),
  /** Unread messages across all chats, for the dock/taskbar badge and the tray. */
  setUnread: (count: number) => ipcRenderer.send("openbot:unread", count),
  /** Called with an in-app path (e.g. "/chats/123") when a notification is clicked. */
  onNavigate: (callback: (path: string) => void) => {
    const listener = (_event: IpcRendererEvent, path: string) => callback(path)
    ipcRenderer.on("openbot:navigate", listener)
    return () => ipcRenderer.removeListener("openbot:navigate", listener)
  },
  /** Opens the screen to connect to a different server. */
  changeServer: () => ipcRenderer.send("openbot:change-server"),
})

// The local connect page only.
contextBridge.exposeInMainWorld("openbotConnect", {
  connect: (address: string): Promise<{ ok: true } | { ok: false; error: string }> =>
    ipcRenderer.invoke("openbot:connect", address),
  info: (): Promise<{ server?: string; error?: string; version: string }> =>
    ipcRenderer.invoke("openbot:connect-info"),
})

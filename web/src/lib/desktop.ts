import { readStorage, writeStorage } from "./storage"

/** What the openbot desktop app (Electron) exposes to the page. Absent in normal browsers. */
export type DesktopBridge = {
  /** "linux" | "darwin" | "win32" */
  platform: string
  /** A native notification; clicking it focuses the window and calls `onNavigate`. */
  notify(notification: { title: string; body: string; chatId?: string; tag?: string }): void
  /** Total unread across chats: dock/taskbar badge, tray tooltip, window title. */
  setUnread(count: number): void
  /** Paths the app wants shown (e.g. from a clicked notification). Returns an unsubscribe. */
  onNavigate(callback: (path: string) => void): () => void
  /** Shows the desktop app's "connect to a server" screen. */
  changeServer(): void
}

declare global {
  interface Window {
    openbotDesktop?: DesktopBridge
  }
}

export function desktop(): DesktopBridge | null {
  return typeof window === "undefined" ? null : (window.openbotDesktop ?? null)
}

export function isDesktop(): boolean {
  return desktop() !== null
}

const NOTIFICATIONS_KEY = "desktop:notifications"

/** The per-device "Desktop notifications" setting (on unless turned off). */
export function desktopNotificationsEnabled(): boolean {
  return readStorage(NOTIFICATIONS_KEY) !== "0"
}

export function setDesktopNotificationsEnabled(on: boolean) {
  writeStorage(NOTIFICATIONS_KEY, on ? "1" : "0")
}

/** Where to get the desktop app. */
export const DESKTOP_DOWNLOAD_URL = "https://github.com/etchebarne/openbot/releases/latest"

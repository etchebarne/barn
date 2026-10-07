/** Namespaced localStorage access that never throws (private mode, blocked storage, SSR). */
const PREFIX = "openbot:"

export function readStorage(key: string): string | null {
  try {
    return window.localStorage.getItem(PREFIX + key)
  } catch {
    return null
  }
}

export function writeStorage(key: string, value: string): void {
  try {
    window.localStorage.setItem(PREFIX + key, value)
  } catch {
    // Storage unavailable: the preference just won't persist.
  }
}

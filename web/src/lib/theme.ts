import { create } from "zustand"

import { readStorage, writeStorage } from "./storage"

export type ThemePreference = "light" | "dark" | "system"
export type ResolvedTheme = "light" | "dark"

/** Must match the inline script in index.html that applies the theme before first paint. */
export const THEME_STORAGE_KEY = "theme"
const DARK_QUERY = "(prefers-color-scheme: dark)"

export function isThemePreference(value: unknown): value is ThemePreference {
  return value === "light" || value === "dark" || value === "system"
}

export function readThemePreference(): ThemePreference {
  const stored = readStorage(THEME_STORAGE_KEY)
  return isThemePreference(stored) ? stored : "system"
}

function systemPrefersDark(): boolean {
  try {
    return window.matchMedia(DARK_QUERY).matches
  } catch {
    return false
  }
}

export function resolveTheme(preference: ThemePreference, prefersDark: boolean): ResolvedTheme {
  if (preference === "system") return prefersDark ? "dark" : "light"
  return preference
}

/**
 * Applies the theme instantly. Theme switching is deliberately not animated, so component
 * color transitions are suppressed for the frame in which the class flips.
 */
export function applyTheme(resolved: ResolvedTheme, root: HTMLElement = document.documentElement) {
  if (root.classList.contains("dark") === (resolved === "dark")) return
  const style = document.createElement("style")
  style.textContent = "*,*::before,*::after{transition:none!important}"
  document.head.append(style)
  root.classList.toggle("dark", resolved === "dark")
  // Force a style recalc with transitions off, then re-enable them.
  void window.getComputedStyle(root).color
  requestAnimationFrame(() => requestAnimationFrame(() => style.remove()))
}

type ThemeState = {
  preference: ThemePreference
  resolved: ResolvedTheme
  setPreference: (preference: ThemePreference) => void
  /** Re-resolve after the OS color scheme changes. */
  syncSystem: () => void
}

export const useThemeStore = create<ThemeState>()((set, get) => {
  const preference = readThemePreference()
  return {
    preference,
    resolved: resolveTheme(preference, systemPrefersDark()),
    setPreference: (next) => {
      writeStorage(THEME_STORAGE_KEY, next)
      const resolved = resolveTheme(next, systemPrefersDark())
      applyTheme(resolved)
      set({ preference: next, resolved })
    },
    syncSystem: () => {
      const resolved = resolveTheme(get().preference, systemPrefersDark())
      applyTheme(resolved)
      set({ resolved })
    },
  }
})

/** Follows OS color scheme changes while the preference is "system". Returns a cleanup. */
export function watchSystemTheme(): () => void {
  let mql: MediaQueryList
  try {
    mql = window.matchMedia(DARK_QUERY)
  } catch {
    return () => {}
  }
  mql.addEventListener("change", onSystemThemeChange)
  return () => mql.removeEventListener("change", onSystemThemeChange)
}

function onSystemThemeChange() {
  useThemeStore.getState().syncSystem()
}

import { useCanGoBack, useRouter } from "@tanstack/react-router"
import { createContext, useContext, type RefObject } from "react"

/** Must match the `md` breakpoint (and hooks/use-mobile). */
export const MOBILE_QUERY = "(max-width: 767px)"

export function isMobileViewport(): boolean {
  try {
    return window.matchMedia(MOBILE_QUERY).matches
  } catch {
    return false
  }
}

/**
 * On desktop `/` opens the first chat; on mobile `/` is the chat list itself, so it never
 * redirects.
 */
export function firstChatRedirect(mobile: boolean, chats: { id: string }[]): string | null {
  if (mobile) return null
  return chats[0]?.id ?? null
}

/* ---- Push/pop animation ---- */

/** Strong ease-out (easing.dev "ease-out-expo"-like), so the screen arrives quickly. */
const PUSH_EASING = "cubic-bezier(0.19, 1, 0.22, 1)"
const PUSH_MS = 250

let lastInput: { kind: "pointer" | "keyboard"; at: number } = { kind: "keyboard", at: 0 }
let lastHistoryTraversal = 0
let listening = false

/** Remembers how the user is navigating, to animate only pointer/touch-driven pushes. */
export function trackNavigationInput() {
  if (listening || typeof window === "undefined") return
  listening = true
  window.addEventListener("pointerdown", () => (lastInput = { kind: "pointer", at: Date.now() }), {
    capture: true,
    passive: true,
  })
  window.addEventListener("keydown", () => (lastInput = { kind: "keyboard", at: Date.now() }), {
    capture: true,
  })
  // Browser back/forward and swipe-back already animate natively.
  window.addEventListener("popstate", () => (lastHistoryTraversal = Date.now()))
}

function reducedMotion() {
  try {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches
  } catch {
    return true
  }
}

/** Whether a screen that just appeared should slide in. */
export function shouldAnimatePush(now = Date.now()): boolean {
  if (reducedMotion()) return false
  if (now - lastHistoryTraversal < 1000) return false
  return lastInput.kind === "pointer" && now - lastInput.at < 1500
}

export function slideIn(element: HTMLElement | null) {
  if (!element || typeof element.animate !== "function") return
  element.animate([{ transform: "translateX(100%)" }, { transform: "translateX(0)" }], {
    duration: PUSH_MS,
    easing: PUSH_EASING,
  })
}

export function slideOut(element: HTMLElement | null): Promise<void> {
  if (!element || typeof element.animate !== "function" || reducedMotion()) {
    return Promise.resolve()
  }
  const animation = element.animate(
    [{ transform: "translateX(0)" }, { transform: "translateX(100%)" }],
    { duration: PUSH_MS, easing: PUSH_EASING, fill: "forwards" },
  )
  // Never hold navigation hostage to the animation (it can stall when the page isn't painting).
  const timeout = new Promise<void>((resolve) => setTimeout(resolve, PUSH_MS + 50))
  return Promise.race([
    animation.finished.then(
      () => undefined,
      () => undefined,
    ),
    timeout,
  ])
}

/** Pushed screens already on their way out. */
const leaving = new WeakSet<HTMLElement>()

/** The pushed screen's element (mobile), so its back button can slide it away. */
export const PushedScreenContext = createContext<RefObject<HTMLElement | null> | null>(null)

/**
 * Back from a pushed screen: slides it out (pointer only), then pops the history entry, or
 * goes to the list when there's nothing to go back to (e.g. a deep link).
 */
export function useScreenBack() {
  const router = useRouter()
  const canGoBack = useCanGoBack()
  const screen = useContext(PushedScreenContext)
  return async (viaPointer: boolean) => {
    const element = screen?.current ?? null
    // A second tap while the screen is leaving would pop a second history entry.
    if (element) {
      if (leaving.has(element)) return
      leaving.add(element)
    }
    if (viaPointer) await slideOut(element)
    if (canGoBack) router.history.back()
    else void router.navigate({ to: "/", replace: true })
  }
}

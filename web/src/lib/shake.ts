/** Small horizontal shake that says "not yet" (e.g. sending an empty message). Uses WAAPI. */
export function shake(element: HTMLElement | null) {
  if (!element || typeof element.animate !== "function") return
  if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return
  element.animate(
    [
      { transform: "translateX(0)" },
      { transform: "translateX(-4px)" },
      { transform: "translateX(4px)" },
      { transform: "translateX(-3px)" },
      { transform: "translateX(2px)" },
      { transform: "translateX(0)" },
    ],
    { duration: 280, easing: "cubic-bezier(0.23, 1, 0.32, 1)" },
  )
}

import { useEffect, useRef, type RefObject } from "react"

/**
 * Escape anywhere inside `ref` calls `onCancel` and stops there, so an inline form closes
 * without also closing the sheet or dialog around it.
 */
export function useEscapeCancel(ref: RefObject<HTMLElement | null>, onCancel: () => void) {
  const cancelRef = useRef(onCancel)
  useEffect(() => {
    cancelRef.current = onCancel
  })
  useEffect(() => {
    const element = ref.current
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return
      event.preventDefault()
      event.stopPropagation()
      cancelRef.current()
    }
    element?.addEventListener("keydown", onKeyDown)
    return () => element?.removeEventListener("keydown", onKeyDown)
  }, [ref])
}

import { create } from "zustand"

type PaletteState = {
  open: boolean
  setOpen: (open: boolean) => void
  /** Bumped to ask the sidebar to start naming a new category. */
  newCategoryRequest: number
  requestNewCategory: () => void
  /** A message to scroll to once its chat is open (a search result). */
  jump: { chatId: string; messageId: string } | null
  requestJump: (chatId: string, messageId: string) => void
  clearJump: () => void
}

/** UI state for the command palette and the actions it triggers elsewhere. */
export const usePaletteStore = create<PaletteState>()((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  newCategoryRequest: 0,
  requestNewCategory: () => set((s) => ({ newCategoryRequest: s.newCategoryRequest + 1 })),
  jump: null,
  requestJump: (chatId, messageId) => set({ jump: { chatId, messageId } }),
  clearJump: () => set({ jump: null }),
}))

/** "⌘K" on Apple platforms, "Ctrl K" elsewhere. */
export function paletteShortcutLabel(
  platform = typeof navigator === "undefined" ? "" : navigator.platform,
): string {
  return /mac|iphone|ipad|ipod/i.test(platform) ? "⌘K" : "Ctrl K"
}

/** Ctrl+K or ⌘+K (without other modifiers). */
export function isPaletteShortcut(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">,
): boolean {
  return (
    event.key.toLowerCase() === "k" &&
    (event.metaKey || event.ctrlKey) &&
    !event.altKey &&
    !event.shiftKey
  )
}

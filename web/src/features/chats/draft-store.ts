import { create } from "zustand"

type DraftState = {
  drafts: Record<string, string>
  setDraft: (chatId: string, text: string) => void
}

/** Unsent composer text per chat, kept while switching chats (UI-only state). */
export const useDraftStore = create<DraftState>()((set) => ({
  drafts: {},
  setDraft: (chatId, text) => set((s) => ({ drafts: { ...s.drafts, [chatId]: text } })),
}))

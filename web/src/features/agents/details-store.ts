import { create } from "zustand"

type DetailsState = {
  /** The agent whose details sheet is open, if any. */
  agentId: string | null
  /** Field to focus when the sheet opens. */
  focus: "model" | null
  open: (agentId: string, options?: { focus?: "model" }) => void
  close: () => void
}

/** The one agent details sheet in the app, opened from the chat header or a failure notice. */
export const useAgentDetailsStore = create<DetailsState>()((set) => ({
  agentId: null,
  focus: null,
  open: (agentId, options) => set({ agentId, focus: options?.focus ?? null }),
  close: () => set({ agentId: null, focus: null }),
}))

export function openAgentDetails(agentId: string, options?: { focus?: "model" }) {
  useAgentDetailsStore.getState().open(agentId, options)
}

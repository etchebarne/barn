import type { Agent } from "@/lib/api-client"

export type TrustMode = Agent["trustMode"]

/** Only granting trust needs confirmation; taking it away is always safe. */
export function trustChangeNeedsConfirmation(from: TrustMode, to: TrustMode): boolean {
  return from !== "trusted" && to === "trusted"
}

export const AUTO_LANGUAGE = "auto"

export function isAutoLanguage(language: string): boolean {
  return language.trim().toLowerCase() === AUTO_LANGUAGE
}

import type { ReactNode } from "react"

import { BrandMark } from "@/components/brand-mark"

/**
 * Centered layout for rare, first-run screens (account setup, login, onboarding).
 * Content enters with a short rise-in; `stepKey` re-triggers it between steps.
 */
export function FirstRunLayout({ children, stepKey }: { children: ReactNode; stepKey?: string }) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-8 bg-muted/40 px-4 py-10">
      <BrandMark className="text-lg" />
      <div key={stepKey} className="w-full max-w-sm animate-rise-in">
        {children}
      </div>
    </div>
  )
}

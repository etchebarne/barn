import { cn } from "cn"

/** openbot's mark: a rounded bot face. Takes the text color. */
export function LogoGlyph({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      aria-hidden="true"
      className={className}
      fill="none"
      stroke="currentColor"
    >
      <rect x="7" y="8.5" width="18" height="16" rx="5.5" strokeWidth={2.6} />
      <path d="M13 14.5v3M19 14.5v3" strokeWidth={2.8} strokeLinecap="round" />
    </svg>
  )
}

/** The openbot wordmark with its mark. */
export function BrandMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 font-semibold tracking-tight select-none",
        className,
      )}
    >
      <LogoGlyph className="size-7" />
      openbot
    </span>
  )
}

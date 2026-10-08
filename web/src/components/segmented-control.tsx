import { cn } from "cn"

/**
 * A small set of mutually exclusive views ("Yours" / "Discover"), drawn as a pill track with
 * the current one raised. Switching is instant: it's a frequent, in-place change.
 */
export function SegmentedControl<T extends string>({
  label,
  value,
  options,
  onChange,
  className,
}: {
  label: string
  value: T
  options: { value: T; label: string; count?: number }[]
  onChange: (value: T) => void
  className?: string
}) {
  return (
    <div
      role="tablist"
      aria-label={label}
      className={cn(
        "inline-flex h-8 items-center gap-0.5 rounded-[10px] bg-muted p-0.5",
        className,
      )}
    >
      {options.map((option) => {
        const selected = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={selected}
            className={cn(
              "flex h-7 items-center gap-1.5 rounded-lg px-3 text-[13px] font-medium outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50",
              selected
                ? "bg-background text-foreground shadow-[0_1px_2px_rgb(0_0_0/12%),0_0_0_1px_var(--border)]"
                : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => onChange(option.value)}
          >
            {option.label}
            {option.count !== undefined && option.count > 0 && (
              <span className="text-xs text-muted-foreground tabular-nums">{option.count}</span>
            )}
          </button>
        )
      })}
    </div>
  )
}

import type { ReactNode } from "react"

/** One settings group: a heading, a short description, and its controls. */
export function SettingsSection({
  id,
  title,
  description,
  children,
}: {
  /** Anchor for deep links, e.g. `/settings?section=models#model-provider`. */
  id?: string
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section id={id} className="flex scroll-mt-4 flex-col gap-4 border-b py-7 last:border-b-0">
      <div className="flex flex-col gap-1">
        <h2 className="text-[15px] font-semibold tracking-tight">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

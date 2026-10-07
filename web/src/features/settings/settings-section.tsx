import type { ReactNode } from "react"

/** One settings group: a heading, a short description, and its controls. */
export function SettingsSection({
  id,
  title,
  description,
  children,
}: {
  /** Anchor for deep links, e.g. `/settings#model-provider`. */
  id?: string
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section id={id} className="flex scroll-mt-4 flex-col gap-4 border-b py-6 last:border-b-0">
      <div className="flex flex-col gap-1">
        <h2 className="text-sm font-medium">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

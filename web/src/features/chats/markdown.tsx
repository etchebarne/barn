import { lazy, Suspense } from "react"

import type { MentionTarget } from "@/lib/mentions"

const MarkdownRenderer = lazy(() => import("./markdown-renderer"))

/**
 * Renders a markdown message body. The parser/highlighter chunk loads on demand; until then
 * the raw text is shown in place so nothing jumps much and nothing is hidden.
 */
export function Markdown({ children, mentions }: { children: string; mentions?: MentionTarget[] }) {
  return (
    <Suspense fallback={<div className="whitespace-pre-wrap">{children}</div>}>
      <MarkdownRenderer mentions={mentions}>{children}</MarkdownRenderer>
    </Suspense>
  )
}

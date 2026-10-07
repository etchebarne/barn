import { useMemo } from "react"
import ReactMarkdown, { type Components, type Options } from "react-markdown"
import rehypeHighlight from "rehype-highlight"
import remarkGfm from "remark-gfm"

import type { MentionTarget } from "@/lib/mentions"

import { rehypeMentions } from "./rehype-mentions"

const components: Components = {
  // Wide tables scroll inside the bubble instead of overflowing it.
  table: ({ node: _node, ...props }) => (
    <div className="table-scroll">
      <table {...props} />
    </div>
  ),
  a: ({ node: _node, children, ...props }) => (
    <a {...props} target="_blank" rel="noreferrer noopener">
      {children}
    </a>
  ),
}

const remarkPlugins: Options["remarkPlugins"] = [remarkGfm]
const NO_MENTIONS: MentionTarget[] = []

/** Markdown renderer for agent messages. Loaded lazily (see `markdown.tsx`). */
export default function MarkdownRenderer({
  children,
  mentions = NO_MENTIONS,
}: {
  children: string
  /** Agents this message mentions; their `@Name`s render as chips. */
  mentions?: MentionTarget[]
}) {
  // Keyed by content so a fresh-but-equal array doesn't re-create the plugin list.
  const mentionKey = mentions.map((m) => `${m.id}:${m.name}`).join("|")
  const rehypePlugins = useMemo<Options["rehypePlugins"]>(
    () => [
      // `detect: false`: only highlight fenced blocks that name their language.
      [rehypeHighlight, { detect: false }],
      [rehypeMentions, { targets: mentions }],
    ],
    // oxlint-disable-next-line react/exhaustive-deps -- `mentionKey` captures `mentions`
    [mentionKey],
  )
  return (
    <div className="markdown">
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={rehypePlugins}
        components={components}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}

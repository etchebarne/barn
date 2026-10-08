import { useMemo } from "react"
import ReactMarkdown, { type Components, type Options } from "react-markdown"
import rehypeHighlight from "rehype-highlight"
import remarkGfm from "remark-gfm"

import { CopyButton } from "@/components/copy-button"
import type { MentionTarget } from "@/lib/mentions"

import { rehypeMentions } from "./rehype-mentions"

type HastNode = {
  type: string
  value?: string
  children?: HastNode[]
  properties?: Record<string, unknown>
}

/** The plain text under a hast node (what a code block copies). */
function hastText(node: HastNode | undefined): string {
  if (!node) return ""
  if (node.type === "text") return node.value ?? ""
  return (node.children ?? []).map(hastText).join("")
}

/** "go" from a `language-go` class on the block's <code>. */
function codeLanguage(node: HastNode | undefined): string | null {
  const code = node?.children?.find((c) => c.type === "element")
  const classes = code?.properties?.className
  const list = Array.isArray(classes) ? classes.map(String) : []
  return list.find((c) => c.startsWith("language-"))?.slice("language-".length) ?? null
}

const components: Components = {
  // Fenced code: a header with the language and a copy button above the scrolling block.
  pre: ({ node, ...props }) => {
    const hast = node as HastNode | undefined
    const language = codeLanguage(hast)
    return (
      <div className="code-block">
        <div className="code-block-header">
          <span>{language ?? "text"}</span>
          <CopyButton text={hastText(hast).replace(/\n$/, "")} label="Copy code" />
        </div>
        <pre {...props} />
      </div>
    )
  },
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

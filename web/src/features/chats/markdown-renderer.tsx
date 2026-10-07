import ReactMarkdown, { type Components, type Options } from "react-markdown"
import rehypeHighlight from "rehype-highlight"
import remarkGfm from "remark-gfm"

const components: Components = {
  a: ({ node: _node, children, ...props }) => (
    <a {...props} target="_blank" rel="noreferrer noopener">
      {children}
    </a>
  ),
}

const remarkPlugins: Options["remarkPlugins"] = [remarkGfm]
// `detect: false`: only highlight fenced blocks that name their language (cheap, predictable).
const rehypePlugins: Options["rehypePlugins"] = [[rehypeHighlight, { detect: false }]]

/** Markdown renderer for agent messages. Loaded lazily (see `markdown.tsx`). */
export default function MarkdownRenderer({ children }: { children: string }) {
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

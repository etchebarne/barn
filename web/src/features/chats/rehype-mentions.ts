import { splitMentions, type MentionTarget } from "@/lib/mentions"

/* Minimal hast shapes (avoids depending on @types/hast directly). */
type HastText = { type: "text"; value: string }
type HastElement = {
  type: "element"
  tagName: string
  properties: Record<string, unknown>
  children: HastNode[]
}
type HastParent = { type: string; children: HastNode[] }
type HastOther = { type: string; children?: HastNode[] }
type HastNode = HastText | HastElement | HastOther

/** Never rewrite inside these (code stays verbatim; links keep their own text). */
const SKIP = new Set(["code", "pre", "a", "kbd", "samp", "script", "style"])

export const MENTION_CLASS = "mention"

function isText(node: HastNode): node is HastText {
  return node.type === "text"
}

function hasChildren(node: HastNode): node is HastParent {
  return "children" in node && Array.isArray(node.children)
}

function isElement(node: HastNode): node is HastElement {
  return node.type === "element" && "tagName" in node
}

function transform(parent: HastParent, targets: MentionTarget[]) {
  const next: HastNode[] = []
  for (const child of parent.children) {
    if (isText(child)) {
      for (const segment of splitMentions(child.value, targets)) {
        if (typeof segment === "string") {
          next.push({ type: "text", value: segment })
        } else {
          next.push({
            type: "element",
            tagName: "span",
            properties: { className: [MENTION_CLASS], dataAgentId: segment.mention.id },
            children: [{ type: "text", value: segment.text }],
          })
        }
      }
      continue
    }
    if (isElement(child) && SKIP.has(child.tagName)) {
      next.push(child)
      continue
    }
    if (hasChildren(child)) transform(child, targets)
    next.push(child)
  }
  parent.children = next
}

/**
 * Rehype plugin: wraps `@Name` text for the message's mentioned agents in a chip span. A text
 * node transform, so markdown structure is untouched and code is skipped.
 */
export function rehypeMentions(options: { targets: MentionTarget[] }) {
  return (tree: HastParent) => {
    if (options.targets.length > 0) transform(tree, options.targets)
  }
}

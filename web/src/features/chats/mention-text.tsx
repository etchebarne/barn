import { splitMentions, type MentionTarget } from "@/lib/mentions"

import { MENTION_CLASS } from "./rehype-mentions"

/** Plain text with the mentioned agents' `@Name`s as chips (user bubbles). */
export function MentionText({ text, mentions }: { text: string; mentions: MentionTarget[] }) {
  const segments = splitMentions(text, mentions)
  return (
    <>
      {segments.map((segment, i) =>
        typeof segment === "string" ? (
          segment
        ) : (
          // oxlint-disable-next-line react/no-array-index-key -- segments are positional
          <span key={i} className={MENTION_CLASS} data-agent-id={segment.mention.id}>
            {segment.text}
          </span>
        ),
      )}
    </>
  )
}

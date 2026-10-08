import { Fragment } from "react"

const URL_PATTERN = /(https?:\/\/[^\s)]+|\b(?:[a-z0-9-]+\.)+[a-z]{2,}\/[^\s)]*)/gi

type Segment = { start: number; text: string; link: boolean }

/** Splits text into plain runs and web addresses, keyed by where they start. */
function segments(text: string): Segment[] {
  const out: Segment[] = []
  let last = 0
  for (const match of text.matchAll(URL_PATTERN)) {
    const start = match.index
    // Trailing punctuation belongs to the sentence, not the address.
    const url = match[0].replace(/[.,;:!?]+$/, "")
    if (start > last) out.push({ start: last, text: text.slice(last, start), link: false })
    out.push({ start, text: url, link: true })
    last = start + url.length
  }
  if (last < text.length) out.push({ start: last, text: text.slice(last), link: false })
  return out
}

/** Plain text with web addresses turned into links (opened in a new tab). */
export function LinkifiedText({ text }: { text: string }) {
  return (
    <>
      {segments(text).map((segment) =>
        segment.link ? (
          <a
            key={segment.start}
            href={/^https?:\/\//i.test(segment.text) ? segment.text : `https://${segment.text}`}
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-4 hover:text-foreground"
          >
            {segment.text}
          </a>
        ) : (
          <Fragment key={segment.start}>{segment.text}</Fragment>
        ),
      )}
    </>
  )
}

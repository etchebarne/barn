import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { useEffect, useState } from "react"

import { api, unwrap, type Schemas } from "@/lib/api-client"

export type SearchResults = Schemas["SearchResults"]
export type MessageHit = Schemas["MessageHit"]

/** Queries shorter than this only filter the palette's own items. */
export const MIN_SEARCH_LENGTH = 2

const MATCH_START = "\u0002"
const MATCH_END = "\u0003"

export type SnippetPart = {
  text: string
  match: boolean
  /** Where it starts in the shown text. */ start: number
}

/** Splits a search snippet into plain and matched parts (see MessageHit.snippet), without Markdown emphasis and code marks. */
export function splitSnippet(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = []
  let start = 0
  const push = (text: string, match: boolean) => {
    if (!text) return
    parts.push({ text, match, start })
    start += text.length
  }
  // Snippets are raw Markdown: drop the markup that reads as noise in one line of text.
  const plain = snippet.replace(/\*\*|__|`+|^#+\s+/g, "")
  for (const chunk of plain.split(MATCH_START)) {
    const end = chunk.indexOf(MATCH_END)
    if (end === -1) {
      push(chunk, false)
      continue
    }
    push(chunk.slice(0, end), true)
    push(chunk.slice(end + 1), false)
  }
  return parts
}

/** `value`, once it has stopped changing for `ms`. */
function useDebounced<T>(value: T, ms: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return debounced
}

/** Messages, memories and tasks matching what's typed in the palette (debounced). */
export function useSearch(query: string) {
  const q = useDebounced(query.trim(), 150)
  return useQuery({
    queryKey: ["search", q],
    queryFn: () => unwrap(api.GET("/search", { params: { query: { q, limit: 20 } } })),
    enabled: q.length >= MIN_SEARCH_LENGTH,
    // Keep showing the last results while the next ones load, so the list doesn't flicker.
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })
}

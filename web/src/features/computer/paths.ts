import type { Schemas } from "@/lib/api-client"

export type FileEntry = Schemas["FileEntry"]
export type FolderListing = Schemas["FolderListing"]

/** "/home/agent" + "notes.md" → "/home/agent/notes.md". */
export function joinPath(dir: string, name: string): string {
  return dir === "/" ? `/${name}` : `${dir}/${name}`
}

/** "/home/agent/notes" → "/home/agent"; "/" stays "/". */
export function parentPath(path: string): string {
  const index = path.lastIndexOf("/")
  return index <= 0 ? "/" : path.slice(0, index)
}

export function baseName(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1) || "/"
}

/** Breadcrumb segments: "/home/agent" → [{"/", "/"}, {"home", "/home"}, {"agent", "/home/agent"}]. */
export function pathSegments(path: string): { name: string; path: string }[] {
  const parts = path.split("/").filter(Boolean)
  return [
    { name: "/", path: "/" },
    ...parts.map((name, i) => ({ name, path: `/${parts.slice(0, i + 1).join("/")}` })),
  ]
}

/**
 * Cleans what the user typed into an absolute path: trims, collapses slashes, resolves "." and
 * "..", and drops a trailing slash. Relative input is taken from `base`. Returns null if empty.
 */
export function normalizePath(input: string, base = "/"): string | null {
  const raw = input.trim()
  if (!raw) return null
  const start = raw.startsWith("/") ? raw : `${base}/${raw}`
  const parts: string[] = []
  for (const part of start.split("/")) {
    if (!part || part === ".") continue
    if (part === "..") parts.pop()
    else parts.push(part)
  }
  return `/${parts.join("/")}`
}

/** Folders (and links to folders) open as folders. */
export function isFolder(entry: Pick<FileEntry, "kind">): boolean {
  return entry.kind === "dir" || entry.kind === "dirlink"
}

export function isHidden(name: string): boolean {
  return name.startsWith(".")
}

/** A valid single file or folder name. */
export function nameError(name: string): string | null {
  const trimmed = name.trim()
  if (!trimmed) return "Enter a name."
  if (trimmed.includes("/")) return "Names can't contain /."
  if (trimmed === "." || trimmed === "..") return "That name is reserved."
  return null
}

const IMAGE = /\.(png|jpe?g|gif|webp|avif|bmp|ico|svg)$/i
const PDF = /\.pdf$/i
const TEXT =
  /\.(txt|md|markdown|json|jsonl|ya?ml|toml|ini|cfg|conf|env|csv|tsv|log|xml|html?|css|scss|js|mjs|cjs|jsx|ts|tsx|py|rb|go|rs|java|kt|c|h|cc|cpp|hpp|cs|php|sh|bash|zsh|fish|sql|lua|r|swift|dockerfile|gitignore|editorconfig|lock)$/i
const TEXT_NAMES =
  /^(makefile|dockerfile|readme|license|changelog|\.[\w.-]+rc|\.bashrc|\.profile)$/i

export type ViewerKind = "image" | "pdf" | "text" | "unknown"

/** A first guess from the name; unknown files are sniffed (see `looksLikeText`). */
export function viewerKindFor(name: string): ViewerKind {
  if (IMAGE.test(name)) return "image"
  if (PDF.test(name)) return "pdf"
  if (TEXT.test(name) || TEXT_NAMES.test(name)) return "text"
  return "unknown"
}

/** Text files over this open as "download only". */
export const MAX_TEXT_BYTES = 2 * 1024 * 1024
/** Uploads over this are refused by the server. */
export const MAX_UPLOAD_BYTES = 200 * 1024 * 1024

/** True when the bytes decode as UTF-8 without NUL bytes (good enough to call it text). */
export function looksLikeText(bytes: Uint8Array): boolean {
  const sample = bytes.subarray(0, 8192)
  if (sample.includes(0)) return false
  try {
    new TextDecoder("utf-8", { fatal: true }).decode(sample)
    return true
  } catch {
    // A multi-byte character cut at the end of the sample isn't a reason to give up.
    if (sample.length === 8192) {
      try {
        new TextDecoder("utf-8", { fatal: true }).decode(sample.subarray(0, 8188))
        return true
      } catch {
        return false
      }
    }
    return false
  }
}

/** Keeps dotfiles out unless asked for. */
export function visibleEntries(entries: FileEntry[], showHidden: boolean): FileEntry[] {
  return showHidden ? entries : entries.filter((e) => !isHidden(e.name))
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
]

/** "3 hours ago", "yesterday", "just now". */
export function timeAgo(iso: string, now: Date = new Date()): string {
  const seconds = (new Date(iso).getTime() - now.getTime()) / 1000
  if (Math.abs(seconds) < 60) return "just now"
  const format = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" })
  for (const [unit, size] of UNITS) {
    if (Math.abs(seconds) >= size) return format.format(Math.round(seconds / size), unit)
  }
  return "just now"
}

export const fullDate = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
})

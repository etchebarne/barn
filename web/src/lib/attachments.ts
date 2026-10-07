import { CSRF_HEADER, type Schemas } from "./api-client"

export type Attachment = Schemas["Attachment"]

/** Per message, matching the server. */
export const MAX_ATTACHMENTS = 10
/** Per file, matching the server (413 above this). */
export const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024

export function isImage(mime: string): boolean {
  return mime.startsWith("image/")
}

/** "512 B", "14 KB", "3.2 MB". */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ["KB", "MB", "GB"]
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value < 10 ? value.toFixed(1).replace(/\.0$/, "") : Math.round(value)} ${units[unit]}`
}

/** Where a download of an attachment goes (the server forces a download with this flag). */
export function downloadUrl(attachment: Pick<Attachment, "url">): string {
  return `${attachment.url}${attachment.url.includes("?") ? "&" : "?"}download=1`
}

/**
 * Which of `files` can be added to a message that already has `existing` attachments: files
 * over 25 MB are rejected, and only as many as fit under the 10-file limit. `error` explains
 * anything left out.
 */
export function acceptFiles(
  existing: number,
  files: File[],
): { accepted: File[]; error: string | null } {
  const errors: string[] = []
  const small = files.filter((file) => {
    if (file.size <= MAX_ATTACHMENT_BYTES) return true
    errors.push(`${file.name} is over 25 MB.`)
    return false
  })
  const room = Math.max(0, MAX_ATTACHMENTS - existing)
  if (small.length > room) {
    errors.push(`You can attach up to ${MAX_ATTACHMENTS} files per message.`)
  }
  return { accepted: small.slice(0, room), error: errors.length > 0 ? errors.join(" ") : null }
}

/** One-line summary for previews when a message has no text: "🖼 Image", "📎 report.pdf". */
export function attachmentSummary(attachments: Attachment[]): string | null {
  if (attachments.length === 0) return null
  if (attachments.every((a) => isImage(a.mime))) {
    return attachments.length === 1 ? "🖼 Image" : `🖼 ${attachments.length} images`
  }
  const file = attachments.find((a) => !isImage(a.mime)) ?? attachments[0]
  const more = attachments.length > 1 ? ` +${attachments.length - 1}` : ""
  return `📎 ${file?.name ?? "File"}${more}`
}

async function errorText(response: Response): Promise<string> {
  if (response.status === 413) return "Files can be up to 25 MB."
  try {
    const body: unknown = await response.json()
    if (body && typeof body === "object" && "message" in body && typeof body.message === "string") {
      return body.message
    }
  } catch {
    // Not JSON.
  }
  return `Upload failed (${response.status})`
}

/**
 * Uploads one file to a chat. Uses fetch directly (multipart), with the CSRF header the
 * server requires on every mutating request.
 */
export async function uploadAttachment(
  chatId: string,
  file: File,
  signal?: AbortSignal,
): Promise<Attachment> {
  const form = new FormData()
  form.append("file", file, file.name)
  const response = await fetch(`/api/chats/${encodeURIComponent(chatId)}/attachments`, {
    method: "POST",
    body: form,
    credentials: "include",
    headers: { [CSRF_HEADER]: "1" },
    signal,
  })
  if (!response.ok) throw new Error(await errorText(response))
  // oxlint-disable-next-line typescript/no-unsafe-type-assertion -- the server's Attachment
  return (await response.json()) as Attachment
}

import { useStore } from "zustand"
import { createStore } from "zustand/vanilla"

import { acceptFiles, isImage, uploadAttachment, type Attachment } from "@/lib/attachments"

/** A file added to the composer, uploading as soon as it's added. */
export type PendingUpload = {
  localId: string
  file: File
  name: string
  mime: string
  size: number
  /** Object URL for image thumbnails while (and after) uploading. */
  previewUrl: string | null
  status: "uploading" | "done" | "error"
  error: string | null
  attachment: Attachment | null
}

type UploadsState = {
  /** Pending uploads per chat (kept when switching chats, like drafts). */
  byChat: Record<string, PendingUpload[]>
  /** Last "couldn't add" message per chat (too big, too many). */
  notice: Record<string, string | null>
}

export const uploadsStore = createStore<UploadsState>()(() => ({ byChat: {}, notice: {} }))

let counter = 0
function nextId() {
  counter += 1
  return `upload-${Date.now().toString(36)}-${counter}`
}

function setItem(chatId: string, localId: string, change: Partial<PendingUpload>) {
  uploadsStore.setState((s) => ({
    byChat: {
      ...s.byChat,
      [chatId]: (s.byChat[chatId] ?? []).map((u) =>
        u.localId === localId ? Object.assign({}, u, change) : u,
      ),
    },
  }))
}

function find(chatId: string, localId: string) {
  return uploadsStore.getState().byChat[chatId]?.find((u) => u.localId === localId)
}

async function run(chatId: string, localId: string) {
  const item = find(chatId, localId)
  if (!item) return
  setItem(chatId, localId, { status: "uploading", error: null })
  try {
    const attachment = await uploadAttachment(chatId, item.file)
    // Removed while uploading: nothing to update.
    if (!find(chatId, localId)) return
    setItem(chatId, localId, { status: "done", attachment })
  } catch (error) {
    if (!find(chatId, localId)) return
    setItem(chatId, localId, {
      status: "error",
      error: error instanceof Error ? error.message : "Upload failed",
    })
  }
}

function createPreviewUrl(file: File): string | null {
  if (!isImage(file.type) || typeof URL.createObjectURL !== "function") return null
  return URL.createObjectURL(file)
}

/** Adds files to a chat's composer (within the limits) and starts uploading them. */
export function addFiles(chatId: string, files: File[]) {
  if (files.length === 0) return
  const existing = uploadsStore.getState().byChat[chatId] ?? []
  const { accepted, error } = acceptFiles(existing.length, files)
  const added: PendingUpload[] = accepted.map((file) => ({
    localId: nextId(),
    file,
    name: file.name || "file",
    mime: file.type || "application/octet-stream",
    size: file.size,
    previewUrl: createPreviewUrl(file),
    status: "uploading",
    error: null,
    attachment: null,
  }))
  uploadsStore.setState((s) => ({
    byChat: { ...s.byChat, [chatId]: [...(s.byChat[chatId] ?? []), ...added] },
    notice: { ...s.notice, [chatId]: error },
  }))
  for (const item of added) void run(chatId, item.localId)
}

export function retryUpload(chatId: string, localId: string) {
  void run(chatId, localId)
}

export function removeUpload(chatId: string, localId: string) {
  const item = find(chatId, localId)
  if (item?.previewUrl) URL.revokeObjectURL(item.previewUrl)
  uploadsStore.setState((s) => ({
    byChat: {
      ...s.byChat,
      [chatId]: (s.byChat[chatId] ?? []).filter((u) => u.localId !== localId),
    },
    notice: { ...s.notice, [chatId]: null },
  }))
}

/**
 * Takes the finished uploads for sending and clears the row. Preview URLs stay alive: the
 * pending bubble shows them until the real message arrives.
 */
export function takeUploads(chatId: string): PendingUpload[] {
  const items = uploadsStore.getState().byChat[chatId] ?? []
  uploadsStore.setState((s) => ({
    byChat: { ...s.byChat, [chatId]: [] },
    notice: { ...s.notice, [chatId]: null },
  }))
  return items.filter((u) => u.status === "done" && u.attachment !== null)
}

const EMPTY: PendingUpload[] = []

export function useUploads(chatId: string): PendingUpload[] {
  return useStore(uploadsStore, (s) => s.byChat[chatId] ?? EMPTY)
}

export function useUploadNotice(chatId: string): string | null {
  return useStore(uploadsStore, (s) => s.notice[chatId] ?? null)
}

/** Send rules: text or a finished upload, and nothing still uploading. */
export function uploadsState(uploads: PendingUpload[]) {
  return {
    uploading: uploads.some((u) => u.status === "uploading"),
    ready: uploads.filter((u) => u.status === "done").length,
  }
}

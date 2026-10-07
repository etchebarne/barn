import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query"

import { api, CSRF_HEADER, unwrap } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

import { MAX_UPLOAD_BYTES } from "./paths"

const base = (agentId: string) => `/api/agents/${encodeURIComponent(agentId)}/sandbox`

export function folderQueryOptions(agentId: string, path: string) {
  return queryOptions({
    queryKey: queryKeys.sandboxFolder(agentId, path),
    queryFn: () =>
      unwrap(
        api.GET("/agents/{agentId}/sandbox/files", {
          params: { path: { agentId }, query: { path } },
        }),
      ),
    staleTime: 0,
    retry: false,
  })
}

/** Where to fetch a file's bytes; `inline` lets images and PDFs show in the page. */
export function fileUrl(agentId: string, path: string, inline = false): string {
  const query = new URLSearchParams({ path })
  if (inline) query.set("inline", "true")
  return `${base(agentId)}/files/download?${query.toString()}`
}

function errorMessage(status: number, text: string): string {
  if (status === 413) return "Files can be up to 200 MB."
  try {
    const body: unknown = JSON.parse(text)
    if (body && typeof body === "object" && "message" in body && typeof body.message === "string") {
      return body.message
    }
  } catch {
    // Not JSON.
  }
  return `Request failed (${status})`
}

/** A file's bytes (for the viewer). */
export async function readFile(agentId: string, path: string, signal?: AbortSignal) {
  const response = await fetch(fileUrl(agentId, path), { credentials: "include", signal })
  if (!response.ok) throw new Error(errorMessage(response.status, await response.text()))
  return new Uint8Array(await response.arrayBuffer())
}

/**
 * Writes a file (upload or save). With `onProgress` it uses XHR, which reports upload
 * progress; fetch can't.
 */
export function writeFile(
  agentId: string,
  path: string,
  body: Blob | string,
  { onProgress, signal }: { onProgress?: (fraction: number) => void; signal?: AbortSignal } = {},
): Promise<void> {
  const size = typeof body === "string" ? new Blob([body]).size : body.size
  if (size > MAX_UPLOAD_BYTES) return Promise.reject(new Error("Files can be up to 200 MB."))
  const url = `${base(agentId)}/files?${new URLSearchParams({ path }).toString()}`
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open("PUT", url)
    xhr.withCredentials = true
    xhr.setRequestHeader(CSRF_HEADER, "1")
    xhr.setRequestHeader("Content-Type", "application/octet-stream")
    xhr.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable) onProgress?.(event.loaded / event.total)
    })
    xhr.addEventListener("load", () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
        return
      }
      reject(new Error(errorMessage(xhr.status, xhr.responseText)))
    })
    xhr.addEventListener("error", () => reject(new Error("Couldn't reach the server.")))
    xhr.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")))
    signal?.addEventListener("abort", () => xhr.abort())
    xhr.send(body)
  })
}

/** Refetches the listed folders after a change. */
function useRefreshFolders(agentId: string) {
  const queryClient = useQueryClient()
  return (...paths: string[]) =>
    Promise.all(
      paths.map((path) =>
        queryClient.invalidateQueries({ queryKey: queryKeys.sandboxFolder(agentId, path) }),
      ),
    )
}

export function useCreateFolder(agentId: string, dir: string) {
  const refresh = useRefreshFolders(agentId)
  return useMutation({
    mutationFn: (path: string) =>
      unwrap(
        api.POST("/agents/{agentId}/sandbox/folders", {
          params: { path: { agentId } },
          body: { path },
        }),
      ),
    onSettled: () => refresh(dir),
  })
}

export function useMoveFile(agentId: string, dir: string) {
  const refresh = useRefreshFolders(agentId)
  return useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      unwrap(
        api.POST("/agents/{agentId}/sandbox/move", {
          params: { path: { agentId } },
          body: { from, to },
        }),
      ),
    onSettled: () => refresh(dir),
  })
}

export function useDeleteFile(agentId: string, dir: string) {
  const refresh = useRefreshFolders(agentId)
  return useMutation({
    mutationFn: (path: string) =>
      unwrap(
        api.DELETE("/agents/{agentId}/sandbox/files", {
          params: { path: { agentId }, query: { path } },
        }),
      ),
    onSettled: () => refresh(dir),
  })
}

export function useWriteFile(agentId: string, dir: string) {
  const refresh = useRefreshFolders(agentId)
  return useMutation({
    mutationFn: ({ path, body }: { path: string; body: Blob | string }) =>
      writeFile(agentId, path, body),
    onSettled: () => refresh(dir),
  })
}

export { useRefreshFolders }

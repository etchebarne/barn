import { cn } from "cn"
import { FileIcon, RotateCwIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { formatBytes } from "@/lib/attachments"

import type { PendingUpload } from "./uploads-store"

function RemoveButton({ upload, onRemove }: { upload: PendingUpload; onRemove: () => void }) {
  return (
    <Button
      type="button"
      variant="secondary"
      size="icon-xs"
      className="absolute -top-1.5 -right-1.5 size-5 rounded-full border shadow-sm [&_svg:not([class*='size-'])]:size-3"
      aria-label={`Remove ${upload.name}`}
      onClick={onRemove}
    >
      <XIcon />
    </Button>
  )
}

function Status({ upload, onRetry }: { upload: PendingUpload; onRetry: () => void }) {
  if (upload.status === "uploading") {
    return <Spinner className="size-3.5" aria-label={`Uploading ${upload.name}`} />
  }
  if (upload.status === "error") {
    return (
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        className="text-destructive"
        aria-label={`Retry uploading ${upload.name}`}
        title={upload.error ?? undefined}
        onClick={onRetry}
      >
        <RotateCwIcon />
      </Button>
    )
  }
  return null
}

/**
 * Files added to the composer, above the textarea: image thumbnails and file chips, each with
 * a spinner while uploading, retry on error, and remove.
 */
export function AttachmentRow({
  uploads,
  notice,
  onRemove,
  onRetry,
}: {
  uploads: PendingUpload[]
  notice: string | null
  onRemove: (localId: string) => void
  onRetry: (localId: string) => void
}) {
  if (uploads.length === 0 && !notice) return null
  return (
    <div className="flex w-full flex-col gap-1.5 px-2.5 pt-2.5">
      {uploads.length > 0 && (
        <ul className="flex flex-wrap gap-2" aria-label="Attachments">
          {uploads.map((upload) => {
            const failed = upload.status === "error"
            return (
              <li key={upload.localId} className="relative">
                {upload.previewUrl ? (
                  <div
                    className={cn(
                      "relative size-14 overflow-hidden rounded-lg border bg-muted",
                      failed && "border-destructive",
                    )}
                  >
                    <img
                      src={upload.previewUrl}
                      alt={upload.name}
                      className={cn(
                        "size-full object-cover",
                        upload.status === "uploading" && "opacity-60",
                      )}
                    />
                    {upload.status !== "done" && (
                      <span className="absolute inset-0 flex items-center justify-center bg-background/40">
                        <Status upload={upload} onRetry={() => onRetry(upload.localId)} />
                      </span>
                    )}
                  </div>
                ) : (
                  <div
                    className={cn(
                      "flex h-14 max-w-56 items-center gap-2 rounded-lg border bg-muted/50 py-1.5 pr-3 pl-2 text-xs",
                      failed && "border-destructive",
                    )}
                  >
                    <FileIcon
                      className="size-4 shrink-0 text-muted-foreground"
                      aria-hidden="true"
                    />
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate font-medium" title={upload.name}>
                        {upload.name}
                      </span>
                      <span className={cn("text-muted-foreground", failed && "text-destructive")}>
                        {failed ? (upload.error ?? "Upload failed") : formatBytes(upload.size)}
                      </span>
                    </span>
                    <Status upload={upload} onRetry={() => onRetry(upload.localId)} />
                  </div>
                )}
                <RemoveButton upload={upload} onRemove={() => onRemove(upload.localId)} />
              </li>
            )
          })}
        </ul>
      )}
      {notice && (
        <p role="alert" className="text-xs text-destructive">
          {notice}
        </p>
      )}
    </div>
  )
}

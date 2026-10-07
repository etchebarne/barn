import { cn } from "cn"
import { DownloadIcon, FileArchiveIcon, FileIcon, FileTextIcon, ImageIcon } from "lucide-react"
import { useState } from "react"

import { buttonVariants } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { downloadUrl, formatBytes, isImage, type Attachment } from "@/lib/attachments"

/** An attachment to show, with a local preview (object URL) while its message is pending. */
export type ShownAttachment = Attachment & { previewUrl?: string | null }

/** Largest a lone image is shown, in CSS pixels. */
const SINGLE_MAX = 320

/** Size a lone image to fit 320px wide, keeping its aspect ratio when the server knows it. */
export function singleImageSize(a: Pick<Attachment, "width" | "height">): {
  width: number
  height: number
} | null {
  if (!a.width || !a.height) return null
  const width = Math.min(SINGLE_MAX, a.width)
  return { width, height: Math.round((width / a.width) * a.height) }
}

/** Images first, then files, each in the order sent. */
export function splitAttachments(attachments: ShownAttachment[]) {
  return {
    images: attachments.filter((a) => isImage(a.mime)),
    files: attachments.filter((a) => !isImage(a.mime)),
  }
}

function FileTypeIcon({ mime }: { mime: string }) {
  const className = "size-5 shrink-0 text-muted-foreground"
  if (mime === "application/pdf" || mime.startsWith("text/")) {
    return <FileTextIcon className={className} aria-hidden="true" />
  }
  if (/zip|tar|gzip|compressed/.test(mime)) {
    return <FileArchiveIcon className={className} aria-hidden="true" />
  }
  if (isImage(mime)) return <ImageIcon className={className} aria-hidden="true" />
  return <FileIcon className={className} aria-hidden="true" />
}

function Lightbox({ image, onClose }: { image: ShownAttachment | null; onClose: () => void }) {
  return (
    <Dialog
      open={image !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      {image && (
        <DialogContent className="flex max-h-[90svh] w-auto max-w-[min(90vw,72rem)] flex-col gap-3 p-3 sm:max-w-[min(90vw,72rem)]">
          <DialogTitle className="truncate pr-10 text-sm">{image.name}</DialogTitle>
          <img
            src={image.previewUrl ?? image.url}
            alt={image.name}
            className="min-h-0 max-w-full flex-1 rounded-md object-contain"
          />
          <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span className="tabular-nums">{formatBytes(image.size)}</span>
            <a
              href={downloadUrl(image)}
              download={image.name}
              className={buttonVariants({ variant: "outline", size: "sm" })}
            >
              <DownloadIcon aria-hidden="true" />
              Download
            </a>
          </div>
        </DialogContent>
      )}
    </Dialog>
  )
}

function ImageThumb({
  image,
  single,
  onOpen,
}: {
  image: ShownAttachment
  single: boolean
  onOpen: () => void
}) {
  const size = single ? singleImageSize(image) : null
  return (
    <button
      type="button"
      className={cn(
        "block overflow-hidden rounded-xl border bg-muted outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50",
        single ? "max-w-80" : "aspect-square w-full",
      )}
      style={
        size ? { width: size.width, aspectRatio: `${size.width} / ${size.height}` } : undefined
      }
      aria-label={`Open ${image.name}`}
      onClick={onOpen}
    >
      <img
        src={image.previewUrl ?? image.url}
        alt={image.name}
        loading="lazy"
        className={cn("block size-full", single ? "object-contain" : "object-cover")}
      />
    </button>
  )
}

function FileCard({ file }: { file: ShownAttachment }) {
  const pdf = file.mime === "application/pdf"
  return (
    <div className="flex w-full max-w-80 items-center gap-3 rounded-xl border bg-background/60 py-2 pr-2 pl-3 text-sm">
      <FileTypeIcon mime={file.mime} />
      <div className="flex min-w-0 flex-1 flex-col">
        {pdf ? (
          <a
            href={file.url}
            target="_blank"
            rel="noopener noreferrer"
            className="truncate font-medium underline-offset-4 hover:underline"
            title={file.name}
          >
            {file.name}
          </a>
        ) : (
          <span className="truncate font-medium" title={file.name}>
            {file.name}
          </span>
        )}
        <span className="text-xs text-muted-foreground tabular-nums">{formatBytes(file.size)}</span>
      </div>
      <a
        href={downloadUrl(file)}
        download={file.name}
        aria-label={`Download ${file.name}`}
        className={buttonVariants({ variant: "ghost", size: "icon-sm" })}
      >
        <DownloadIcon aria-hidden="true" />
      </a>
    </div>
  )
}

/**
 * A message's attachments: images first (one larger, several in a grid; click to open), then
 * file cards with a download link.
 */
export function MessageAttachments({
  attachments,
  align,
}: {
  attachments: ShownAttachment[]
  align: "start" | "end"
}) {
  const [open, setOpen] = useState<ShownAttachment | null>(null)
  if (attachments.length === 0) return null
  const { images, files } = splitAttachments(attachments)
  const single = images.length === 1

  return (
    <div
      className={cn(
        "flex w-full min-w-0 flex-col gap-1.5",
        align === "end" ? "items-end" : "items-start",
      )}
    >
      {images.length > 0 && (
        <ul
          aria-label="Images"
          className={cn(
            single ? "flex" : "grid w-full max-w-80 gap-1",
            !single && (images.length === 2 || images.length === 4 ? "grid-cols-2" : "grid-cols-3"),
          )}
        >
          {images.map((image) => (
            <li key={image.id} className="min-w-0">
              <ImageThumb image={image} single={single} onOpen={() => setOpen(image)} />
            </li>
          ))}
        </ul>
      )}
      {files.length > 0 && (
        <ul
          aria-label="Files"
          className={cn(
            "flex w-full flex-col gap-1.5",
            align === "end" ? "items-end" : "items-start",
          )}
        >
          {files.map((file) => (
            <li key={file.id} className="w-full max-w-80">
              <FileCard file={file} />
            </li>
          ))}
        </ul>
      )}
      <Lightbox image={open} onClose={() => setOpen(null)} />
    </div>
  )
}

import { PaperclipIcon } from "lucide-react"
import { useRef, useState, type DragEvent, type ReactNode } from "react"

function hasFiles(event: DragEvent) {
  return [...event.dataTransfer.types].includes("Files")
}

/** The chat area: drop files anywhere on it to attach them, with a subtle overlay while dragging. */
export function FileDropZone({
  onFiles,
  children,
}: {
  onFiles: (files: File[]) => void
  children: ReactNode
}) {
  const [dragging, setDragging] = useState(false)
  // dragenter/dragleave fire for every child; count them to know when the drag really leaves.
  const depth = useRef(0)

  return (
    <div
      className="relative flex h-svh min-w-0 flex-1 flex-col"
      onDragEnter={(event) => {
        if (!hasFiles(event)) return
        event.preventDefault()
        depth.current += 1
        setDragging(true)
      }}
      onDragOver={(event) => {
        if (!hasFiles(event)) return
        event.preventDefault()
        event.dataTransfer.dropEffect = "copy"
      }}
      onDragLeave={(event) => {
        if (!hasFiles(event)) return
        depth.current = Math.max(0, depth.current - 1)
        if (depth.current === 0) setDragging(false)
      }}
      onDrop={(event) => {
        if (!hasFiles(event)) return
        event.preventDefault()
        depth.current = 0
        setDragging(false)
        onFiles([...event.dataTransfer.files])
      }}
    >
      {children}
      {dragging && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-2 z-40 flex items-center justify-center rounded-2xl border-2 border-dashed border-primary/40 bg-background/80 text-sm font-medium"
        >
          <span className="flex items-center gap-2">
            <PaperclipIcon className="size-4" />
            Drop files to attach
          </span>
        </div>
      )}
    </div>
  )
}

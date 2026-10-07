import { useQuery } from "@tanstack/react-query"
import { cn } from "cn"
import { ChevronLeftIcon, DownloadIcon, XIcon } from "lucide-react"
import { useRef, useState, type KeyboardEvent } from "react"
import { toast } from "sonner"

import { Button, buttonVariants } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { formatBytes } from "@/lib/attachments"

import { fileUrl, readFile, writeFile } from "./api"
import { baseName, looksLikeText, MAX_TEXT_BYTES, viewerKindFor, type FileEntry } from "./paths"
import { MONO_FONT } from "./terminal-session"

const decoder = new TextDecoder()

/** A plain-text editor with line numbers (no wrapping, so the numbers line up). */
function TextEditor({
  initial,
  label,
  onDirtyChange,
  onSave,
}: {
  initial: string
  label: string
  onDirtyChange: (dirty: boolean) => void
  onSave: (text: string) => Promise<void>
}) {
  const [saved, setSaved] = useState(initial)
  const [text, setText] = useState(initial)
  const [saving, setSaving] = useState(false)
  const gutterRef = useRef<HTMLDivElement>(null)
  const dirty = text !== saved
  const lines = text.split("\n").length

  async function save() {
    if (!dirty || saving) return
    setSaving(true)
    const snapshot = text
    try {
      await onSave(snapshot)
      setSaved(snapshot)
      onDirtyChange(snapshot !== text)
    } catch (error) {
      toast.error(`Couldn't save: ${error instanceof Error ? error.message : String(error)}`)
    } finally {
      setSaving(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
      event.preventDefault()
      void save()
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b px-3 py-1.5 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5" aria-live="polite">
          {dirty ? (
            <>
              <span className="size-1.5 rounded-full bg-amber-500" aria-hidden="true" />
              Unsaved changes
            </>
          ) : (
            "Saved"
          )}
        </span>
        <span className="ml-auto hidden sm:inline">
          {navigator.platform.includes("Mac") ? "⌘S" : "Ctrl+S"} to save
        </span>
        <Button
          size="xs"
          className="ml-auto sm:ml-0"
          disabled={!dirty || saving}
          onClick={() => void save()}
        >
          {saving && <Spinner />}
          Save
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 text-[13px] leading-5" style={{ fontFamily: MONO_FONT }}>
        <div
          ref={gutterRef}
          aria-hidden="true"
          className="shrink-0 overflow-hidden border-r bg-muted/40 py-2 pr-2 pl-3 text-right text-muted-foreground tabular-nums select-none"
        >
          {Array.from({ length: lines }, (_, i) => (
            <div key={i}>{i + 1}</div>
          ))}
        </div>
        <textarea
          aria-label={label}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          wrap="off"
          value={text}
          className="min-h-0 flex-1 resize-none overflow-auto bg-transparent px-3 py-2 whitespace-pre outline-none"
          onChange={(event) => {
            setText(event.target.value)
            onDirtyChange(event.target.value !== saved)
          }}
          onKeyDown={onKeyDown}
          onScroll={(event) => {
            if (gutterRef.current) gutterRef.current.scrollTop = event.currentTarget.scrollTop
          }}
        />
      </div>
    </div>
  )
}

function DownloadOnly({
  agentId,
  path,
  reason,
}: {
  agentId: string
  path: string
  reason: string
}) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground">
      <p>{reason}</p>
      <a
        href={fileUrl(agentId, path)}
        download={baseName(path)}
        className={buttonVariants({ variant: "outline", size: "sm" })}
      >
        <DownloadIcon aria-hidden="true" />
        Download
      </a>
    </div>
  )
}

/** Text files load their bytes; anything that isn't UTF-8 falls back to download. */
function TextOrBinary({
  agentId,
  path,
  onDirtyChange,
  onSaved,
}: {
  agentId: string
  path: string
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
}) {
  const { data, error, isPending } = useQuery({
    queryKey: ["sandbox-file", agentId, path],
    queryFn: ({ signal }) => readFile(agentId, path, signal),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  })
  if (isPending) {
    return (
      <div className="flex flex-1 items-center justify-center text-muted-foreground">
        <Spinner />
      </div>
    )
  }
  if (error) {
    return <p className="p-4 text-sm text-destructive">Couldn't open the file: {error.message}</p>
  }
  if (!looksLikeText(data)) {
    return <DownloadOnly agentId={agentId} path={path} reason="This file can't be shown here." />
  }
  return (
    <TextEditor
      initial={decoder.decode(data)}
      label={`Contents of ${baseName(path)}`}
      onDirtyChange={onDirtyChange}
      onSave={async (text) => {
        await writeFile(agentId, path, text)
        onSaved()
      }}
    />
  )
}

/**
 * Shows an open file: images and PDFs inline, text in an editor, anything else as a download.
 * A side panel on desktop, the whole screen on phones.
 */
export function FileViewer({
  agentId,
  path,
  entry,
  mobile,
  confirmingDiscard,
  onDirtyChange,
  onSaved,
  onClose,
  onKeepEditing,
  onDiscard,
}: {
  agentId: string
  path: string
  entry: FileEntry | undefined
  mobile: boolean
  /** Closing or switching files with unsaved changes asks first. */
  confirmingDiscard: boolean
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
  onClose: () => void
  onKeepEditing: () => void
  onDiscard: () => void
}) {
  const name = baseName(path)
  const kind = viewerKindFor(name)
  const size = entry?.size ?? 0
  const tooBig = (kind === "text" || kind === "unknown") && size > MAX_TEXT_BYTES

  return (
    <section
      aria-label={name}
      className={cn(
        "flex min-h-0 min-w-0 flex-col bg-background",
        mobile ? "absolute inset-0 z-20" : "w-1/2 max-w-[48rem] border-l",
      )}
    >
      <div className="flex h-11 shrink-0 items-center gap-2 border-b px-2">
        {mobile && (
          <Button variant="ghost" size="icon-sm" aria-label="Back to files" onClick={onClose}>
            <ChevronLeftIcon />
          </Button>
        )}
        <div className="flex min-w-0 flex-1 flex-col pl-1">
          <span className="truncate text-sm font-medium">{name}</span>
          <span className="truncate text-xs text-muted-foreground">
            {path}
            {entry ? ` · ${formatBytes(entry.size)}` : ""}
          </span>
        </div>
        <a
          href={fileUrl(agentId, path)}
          download={name}
          aria-label={`Download ${name}`}
          className={buttonVariants({ variant: "ghost", size: "icon-sm" })}
        >
          <DownloadIcon aria-hidden="true" />
        </a>
        {!mobile && (
          <Button variant="ghost" size="icon-sm" aria-label="Close file" onClick={onClose}>
            <XIcon />
          </Button>
        )}
      </div>
      {confirmingDiscard && (
        <div
          role="alertdialog"
          aria-label="Discard unsaved changes?"
          className="flex shrink-0 flex-wrap items-center gap-2 border-b border-destructive/30 bg-destructive/5 px-3 py-2 text-sm"
        >
          <p className="flex-1">Discard your unsaved changes to {name}?</p>
          <Button size="xs" variant="ghost" autoFocus onClick={onKeepEditing}>
            Keep editing
          </Button>
          <Button size="xs" variant="destructive" onClick={onDiscard}>
            Discard
          </Button>
        </div>
      )}
      {kind === "image" ? (
        <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto bg-muted/30 p-4">
          <img
            src={fileUrl(agentId, path, true)}
            alt={name}
            className="max-h-full max-w-full rounded-md object-contain"
          />
        </div>
      ) : kind === "pdf" ? (
        <object
          data={fileUrl(agentId, path, true)}
          type="application/pdf"
          aria-label={name}
          className="min-h-0 w-full flex-1 bg-white"
        >
          <DownloadOnly agentId={agentId} path={path} reason="This browser can't show PDFs here." />
        </object>
      ) : tooBig ? (
        <DownloadOnly
          agentId={agentId}
          path={path}
          reason={`${name} is ${formatBytes(size)}; files over 2 MB can't be edited here.`}
        />
      ) : (
        <TextOrBinary
          key={path}
          agentId={agentId}
          path={path}
          onDirtyChange={onDirtyChange}
          onSaved={onSaved}
        />
      )}
    </section>
  )
}

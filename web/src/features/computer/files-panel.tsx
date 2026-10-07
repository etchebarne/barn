import { useQuery, useQueryClient } from "@tanstack/react-query"
import { cn } from "cn"
import {
  EllipsisIcon,
  EyeIcon,
  EyeOffIcon,
  FileArchiveIcon,
  FileCodeIcon,
  FileIcon,
  FileImageIcon,
  FilePlusIcon,
  FileQuestionMarkIcon,
  FileSymlinkIcon,
  FileTextIcon,
  FolderIcon,
  FolderPlusIcon,
  FolderSymlinkIcon,
  HouseIcon,
  RefreshCwIcon,
  UploadIcon,
  UsersIcon,
  XIcon,
} from "lucide-react"
import {
  useEffect,
  useId,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
} from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { FieldError } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useIsMobile } from "@/hooks/use-mobile"
import { ApiError } from "@/lib/api-client"
import { formatBytes } from "@/lib/attachments"
import { queryKeys } from "@/lib/query-keys"
import { SANDBOX_HOME } from "@/lib/sandbox"
import { readStorage, writeStorage } from "@/lib/storage"

import {
  fileUrl,
  folderQueryOptions,
  useCreateFolder,
  useDeleteFile,
  useMoveFile,
  useRefreshFolders,
  writeFile,
} from "./api"
import { FileViewer } from "./file-viewer"
import {
  baseName,
  fullDate,
  isFolder,
  joinPath,
  nameError,
  normalizePath,
  parentPath,
  pathSegments,
  timeAgo,
  viewerKindFor,
  visibleEntries,
  type FileEntry,
} from "./paths"

const SHOW_HIDDEN_KEY = "computer:show-hidden"
const SHARED = "/shared"

function EntryIcon({ entry }: { entry: FileEntry }) {
  const className = "size-4 shrink-0"
  if (entry.kind === "dir") return <FolderIcon className={cn(className, "text-sky-500")} />
  if (entry.kind === "dirlink")
    return <FolderSymlinkIcon className={cn(className, "text-sky-500")} />
  if (entry.kind === "link") return <FileSymlinkIcon className={className} />
  if (entry.kind === "other") return <FileQuestionMarkIcon className={className} />
  const kind = viewerKindFor(entry.name)
  if (kind === "image") return <FileImageIcon className={className} />
  if (kind === "pdf") return <FileTextIcon className={className} />
  if (/\.(zip|tar|gz|tgz|bz2|xz|7z|rar)$/i.test(entry.name))
    return <FileArchiveIcon className={className} />
  if (kind === "text") return <FileCodeIcon className={className} />
  return <FileIcon className={className} />
}

type Upload = { id: string; name: string; progress: number; error: string | null }

/** Breadcrumb that turns into a path field on click (or with the edit button). */
function PathBar({ path, onNavigate }: { path: string; onNavigate: (path: string) => void }) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(path)
  if (editing) {
    return (
      <form
        className="min-w-0 flex-1"
        onSubmit={(event) => {
          event.preventDefault()
          const next = normalizePath(value, path)
          setEditing(false)
          if (next) onNavigate(next)
        }}
      >
        <Input
          aria-label="Folder path"
          autoFocus
          spellCheck={false}
          value={value}
          className="h-7 font-mono text-xs"
          onChange={(event) => setValue(event.target.value)}
          onFocus={(event) => event.currentTarget.select()}
          onBlur={() => setEditing(false)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault()
              setEditing(false)
            }
          }}
        />
      </form>
    )
  }
  const segments = pathSegments(path)
  return (
    <nav aria-label="Folder path" className="flex min-w-0 flex-1 items-center">
      <ol
        // Long paths scroll, showing their end (the current folder) first.
        ref={(list) => {
          if (list) list.scrollLeft = list.scrollWidth
        }}
        className="flex min-w-0 [scrollbar-width:none] items-center overflow-x-auto text-sm [&::-webkit-scrollbar]:hidden"
      >
        {segments.map((segment, i) => (
          <li key={segment.path} className="flex shrink-0 items-center">
            {i > 1 && (
              <span className="px-0.5 text-muted-foreground" aria-hidden="true">
                /
              </span>
            )}
            <button
              type="button"
              className={cn(
                "rounded px-1 py-0.5 outline-none select-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50",
                i === segments.length - 1 ? "font-medium" : "text-muted-foreground",
              )}
              aria-current={i === segments.length - 1 ? "location" : undefined}
              onClick={() => onNavigate(segment.path)}
            >
              {segment.name}
            </button>
          </li>
        ))}
      </ol>
      {/* The empty space after the crumbs edits the path. */}
      <button
        type="button"
        aria-label="Edit path"
        className="h-7 min-w-8 flex-1 cursor-text rounded outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        onClick={() => {
          setValue(path)
          setEditing(true)
        }}
      />
    </nav>
  )
}

/** A name field in the list (rename, new folder, new file). Enter saves, Escape cancels. */
function NameField({
  initial,
  label,
  icon,
  onSubmit,
  onCancel,
}: {
  initial: string
  label: string
  icon: ReactNode
  onSubmit: (name: string) => void
  onCancel: () => void
}) {
  const [value, setValue] = useState(initial)
  const [error, setError] = useState<string | null>(null)
  return (
    <div className="flex items-center gap-3 px-3 py-1">
      {icon}
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <Input
          aria-label={label}
          autoFocus
          spellCheck={false}
          value={value}
          aria-invalid={!!error || undefined}
          className="h-7"
          onFocus={(event) => {
            // Select the name without its extension, like file managers do.
            const dot = initial.lastIndexOf(".")
            event.currentTarget.setSelectionRange(0, dot > 0 ? dot : initial.length)
          }}
          onChange={(event) => {
            setValue(event.target.value)
            setError(null)
          }}
          onBlur={onCancel}
          onKeyDown={(event) => {
            event.stopPropagation()
            if (event.key === "Enter") {
              event.preventDefault()
              const problem = nameError(value)
              if (problem) setError(problem)
              else if (value.trim() === initial) onCancel()
              else onSubmit(value.trim())
            } else if (event.key === "Escape") {
              event.preventDefault()
              onCancel()
            }
          }}
        />
        {error && <FieldError>{error}</FieldError>}
      </div>
    </div>
  )
}

function MoveDialog({
  entryPath,
  pending,
  error,
  onMove,
  onClose,
}: {
  entryPath: string | null
  pending: boolean
  error: string | undefined
  onMove: (to: string) => void
  onClose: () => void
}) {
  const [value, setValue] = useState(entryPath ?? "")
  const [shown, setShown] = useState(entryPath)
  if (entryPath !== shown) {
    setShown(entryPath)
    setValue(entryPath ?? "")
  }
  const name = entryPath ? baseName(entryPath) : ""
  return (
    <Dialog open={entryPath !== null} onOpenChange={(open) => !open && !pending && onClose()}>
      <DialogContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            const to = normalizePath(value)
            if (to && entryPath && to !== entryPath) onMove(to)
          }}
        >
          <DialogHeader>
            <DialogTitle>Move {name}</DialogTitle>
            <DialogDescription>Its new full path, including the name.</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-1.5">
            <Input
              aria-label="New path"
              autoFocus
              spellCheck={false}
              className="font-mono"
              value={value}
              aria-invalid={!!error || undefined}
              onChange={(event) => setValue(event.target.value)}
            />
            {error && <FieldError>{error}</FieldError>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={pending || !normalizePath(value)}>
              {pending && <Spinner />}
              Move
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DeleteDialog({
  entry,
  pending,
  onDelete,
  onClose,
}: {
  entry: FileEntry | null
  pending: boolean
  onDelete: () => void
  onClose: () => void
}) {
  // Keep the last entry while the dialog animates out.
  const [shown, setShown] = useState(entry)
  if (entry && entry !== shown) setShown(entry)
  const folder = shown ? isFolder(shown) : false
  return (
    <Dialog open={entry !== null} onOpenChange={(open) => !open && !pending && onClose()}>
      <DialogContent showCloseButton={false} role="alertdialog">
        <DialogHeader>
          <DialogTitle>Delete {shown?.name}?</DialogTitle>
          <DialogDescription>
            {folder
              ? `This deletes the folder ${shown?.name} and everything in it. This can't be undone.`
              : "This can't be undone."}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" disabled={pending} autoFocus onClick={onClose}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={pending} onClick={onDelete}>
            {pending && <Spinner />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Starts a download the way a link with `download` would. */
function downloadFile(url: string, name: string) {
  const link = document.createElement("a")
  link.href = url
  link.download = name
  link.click()
}

function folderError(error: Error): string {
  if (error instanceof ApiError && error.status === 404) return "This folder doesn't exist."
  if (error instanceof ApiError && error.status === 503) {
    return "The computer isn't reachable right now (Docker isn't available)."
  }
  return `Couldn't open this folder: ${error.message}`
}

/**
 * The Files tab: browse the agent's computer, open files, upload (button or drop), and
 * create, rename, move or delete. The list works with the keyboard: arrows, Enter to open,
 * Backspace to go up, Delete to delete, F2 to rename, Shift+F10 for the menu.
 */
export function FilesPanel({ agentId }: { agentId: string }) {
  const mobile = useIsMobile()
  const listId = useId()
  const [path, setPath] = useState(SANDBOX_HOME)
  const [showHidden, setShowHidden] = useState(() => readStorage(SHOW_HIDDEN_KEY) === "1")
  const [selected, setSelected] = useState<string | null>(null)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [creating, setCreating] = useState<"folder" | "file" | null>(null)
  const [menuFor, setMenuFor] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<FileEntry | null>(null)
  const [moving, setMoving] = useState<string | null>(null)
  const [uploads, setUploads] = useState<Upload[]>([])
  const [dragging, setDragging] = useState(false)
  const [open, setOpen] = useState<{ path: string; entry: FileEntry } | null>(null)
  const [dirty, setDirty] = useState(false)
  const [pendingNav, setPendingNav] = useState<(() => void) | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const lastPointer = useRef<string>("mouse")

  const folder = useQuery(folderQueryOptions(agentId, path))
  const refresh = useRefreshFolders(agentId)
  const createFolder = useCreateFolder(agentId, path)
  const move = useMoveFile(agentId, path)
  const remove = useDeleteFile(agentId, path)

  // Listing starts the computer if it was stopped: refresh its status once it answers.
  const queryClient = useQueryClient()
  const loaded = folder.isSuccess
  useEffect(() => {
    if (loaded) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.sandbox(agentId), exact: true })
    }
  }, [loaded, queryClient, agentId])

  const entries = folder.data ? visibleEntries(folder.data.entries, showHidden) : []
  const selectedIndex = entries.findIndex((e) => e.name === selected)
  const rowId = (index: number) => `${listId}-row-${index}`

  useEffect(() => {
    if (selectedIndex < 0) return
    document.getElementById(rowId(selectedIndex))?.scrollIntoView({ block: "nearest" })
    // rowId only depends on listId.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedIndex])

  /** Runs `action` now, or after confirming that unsaved edits can go. */
  function guard(action: () => void) {
    if (open && dirty) setPendingNav(() => action)
    else action()
  }

  function navigate(next: string) {
    setPath(next)
    setSelected(null)
    setRenaming(null)
    setCreating(null)
  }

  function openEntry(entry: FileEntry) {
    const target = joinPath(path, entry.name)
    if (isFolder(entry)) {
      navigate(target)
      return
    }
    guard(() => {
      setDirty(false)
      setOpen({ path: target, entry })
    })
  }

  function goUp() {
    if (path === "/") return
    const from = baseName(path)
    navigate(parentPath(path))
    setSelected(from)
  }

  async function upload(files: File[]) {
    const dir = path
    await Promise.all(
      files.map(async (file) => {
        const id = `${Date.now()}-${Math.random()}`
        setUploads((list) => [...list, { id, name: file.name, progress: 0, error: null }])
        try {
          await writeFile(agentId, joinPath(dir, file.name), file, {
            onProgress: (progress) =>
              setUploads((list) => list.map((u) => (u.id === id ? { ...u, progress } : u))),
          })
          setUploads((list) => list.filter((u) => u.id !== id))
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error)
          setUploads((list) => list.map((u) => (u.id === id ? { ...u, error: message } : u)))
        }
      }),
    )
    void refresh(dir)
  }

  function rename(entry: FileEntry, name: string) {
    setRenaming(null)
    const from = joinPath(path, entry.name)
    const to = joinPath(path, name)
    move.mutate(
      { from, to },
      {
        onSuccess: () => {
          setSelected(name)
          if (open?.path === from) setOpen({ path: to, entry: { ...entry, name } })
        },
        onError: (error) => toast.error(`Couldn't rename ${entry.name}: ${error.message}`),
      },
    )
    listRef.current?.focus()
  }

  async function create(kind: "folder" | "file", name: string) {
    setCreating(null)
    const target = joinPath(path, name)
    try {
      if (kind === "folder") {
        await createFolder.mutateAsync(target)
      } else {
        await writeFile(agentId, target, "")
        await refresh(path)
        const entry: FileEntry = {
          name,
          kind: "file",
          size: 0,
          modifiedAt: new Date().toISOString(),
        }
        guard(() => {
          setDirty(false)
          setOpen({ path: target, entry })
        })
      }
      setSelected(name)
    } catch (error) {
      toast.error(
        `Couldn't create ${name}: ${error instanceof Error ? error.message : String(error)}`,
      )
    }
    listRef.current?.focus()
  }

  function rowFromEvent(event: MouseEvent<HTMLElement>): FileEntry | undefined {
    const target = event.target
    if (!(target instanceof Element)) return undefined
    const row = target.closest<HTMLElement>("[data-entry]")
    return row ? entries.find((e) => e.name === row.dataset.entry) : undefined
  }

  function onListKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget) return
    const index = selectedIndex
    const entry = entries[index]
    const select = (i: number) => {
      const next = entries[Math.max(0, Math.min(entries.length - 1, i))]
      if (next) setSelected(next.name)
    }
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault()
        select(index < 0 ? 0 : index + 1)
        break
      case "ArrowUp":
        event.preventDefault()
        select(index < 0 ? entries.length - 1 : index - 1)
        break
      case "Home":
        event.preventDefault()
        select(0)
        break
      case "End":
        event.preventDefault()
        select(entries.length - 1)
        break
      case "Enter":
        if (entry) {
          event.preventDefault()
          openEntry(entry)
        }
        break
      case "Backspace":
        event.preventDefault()
        goUp()
        break
      case "Delete":
        if (entry) {
          event.preventDefault()
          setDeleting(entry)
        }
        break
      case "F2":
        if (entry) {
          event.preventDefault()
          setRenaming(entry.name)
        }
        break
      case "ContextMenu":
        if (entry) {
          event.preventDefault()
          setMenuFor(entry.name)
        }
        break
      default:
        if (event.key === "F10" && event.shiftKey && entry) {
          event.preventDefault()
          setMenuFor(entry.name)
        }
    }
  }

  function onDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    setDragging(false)
    const files = Array.from(event.dataTransfer.files)
    if (files.length > 0) void upload(files)
  }

  const viewerOpen = open !== null
  const now = new Date()

  return (
    <div className="relative flex h-full min-h-0">
      <div
        className={cn("flex min-h-0 min-w-0 flex-1 flex-col", mobile && viewerOpen && "invisible")}
      >
        {/* Toolbar */}
        <div className="flex shrink-0 flex-wrap items-center gap-1 border-b px-2 py-1.5">
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Home folder"
                  onClick={() => navigate(SANDBOX_HOME)}
                />
              }
            >
              <HouseIcon />
            </TooltipTrigger>
            <TooltipContent>{SANDBOX_HOME}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Shared folder"
                  className={cn(path === SHARED && "bg-muted")}
                  onClick={() => navigate(SHARED)}
                />
              }
            >
              <UsersIcon />
            </TooltipTrigger>
            <TooltipContent>{SHARED}: in every agent's computer</TooltipContent>
          </Tooltip>
          <span aria-hidden="true" className="mx-1 h-4 w-px shrink-0 bg-border max-sm:hidden" />
          <div className="order-last flex w-full min-w-0 items-center sm:order-none sm:w-auto sm:flex-1">
            <PathBar key={path} path={path} onNavigate={navigate} />
          </div>
          <div className="ml-auto flex items-center gap-0.5">
            <input
              ref={fileInputRef}
              type="file"
              multiple
              hidden
              onChange={(event) => {
                const files = Array.from(event.target.files ?? [])
                event.target.value = ""
                if (files.length > 0) void upload(files)
              }}
            />
            <Button variant="ghost" size="sm" onClick={() => fileInputRef.current?.click()}>
              <UploadIcon />
              <span className="max-sm:sr-only">Upload</span>
            </Button>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="New folder"
                    onClick={() => setCreating("folder")}
                  />
                }
              >
                <FolderPlusIcon />
              </TooltipTrigger>
              <TooltipContent>New folder</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="New file"
                    onClick={() => setCreating("file")}
                  />
                }
              >
                <FilePlusIcon />
              </TooltipTrigger>
              <TooltipContent>New file</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={showHidden ? "Hide hidden files" : "Show hidden files"}
                    aria-pressed={showHidden}
                    onClick={() => {
                      writeStorage(SHOW_HIDDEN_KEY, showHidden ? "0" : "1")
                      setShowHidden(!showHidden)
                    }}
                  />
                }
              >
                {showHidden ? <EyeIcon /> : <EyeOffIcon />}
              </TooltipTrigger>
              <TooltipContent>
                {showHidden ? "Hide hidden files" : "Show hidden files"}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Refresh"
                    disabled={folder.isFetching}
                    onClick={() => void folder.refetch()}
                  />
                }
              >
                <RefreshCwIcon />
              </TooltipTrigger>
              <TooltipContent>Refresh</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {uploads.length > 0 && (
          <ul
            aria-label="Uploads"
            className="flex shrink-0 flex-col gap-1 border-b px-3 py-2 text-xs"
          >
            {uploads.map((u) => (
              <li key={u.id} className="flex items-center gap-2">
                <span className="min-w-0 flex-1 truncate">{u.name}</span>
                {u.error ? (
                  <>
                    <span className="text-destructive">{u.error}</span>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      aria-label={`Dismiss ${u.name}`}
                      onClick={() => setUploads((list) => list.filter((x) => x.id !== u.id))}
                    >
                      <XIcon />
                    </Button>
                  </>
                ) : (
                  <progress
                    aria-label={`Uploading ${u.name}`}
                    value={Math.round(u.progress * 100)}
                    max={100}
                    className="h-1.5 w-24 overflow-hidden rounded-full [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-primary"
                  />
                )}
              </li>
            ))}
          </ul>
        )}

        {folder.data?.truncated && (
          <p className="shrink-0 border-b bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
            Showing the first {folder.data.entries.length.toLocaleString()} items. Use the terminal
            to see the rest.
          </p>
        )}

        {/* The list (and drop zone) */}
        <div
          ref={listRef}
          id={listId}
          role="listbox"
          tabIndex={0}
          aria-label={`Files in ${path}`}
          aria-activedescendant={selectedIndex >= 0 ? rowId(selectedIndex) : undefined}
          className={cn(
            "relative min-h-0 flex-1 overflow-y-auto py-1 outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:ring-inset",
            dragging && "bg-primary/5 ring-2 ring-primary/40 ring-inset",
          )}
          onKeyDown={onListKeyDown}
          onPointerDown={(event) => {
            lastPointer.current = event.pointerType
          }}
          onClick={(event) => {
            const entry = rowFromEvent(event)
            if (!entry) return
            setSelected(entry.name)
            // Touch has no double tap convention here: a tap opens.
            if (lastPointer.current === "touch") openEntry(entry)
          }}
          onDoubleClick={(event) => {
            const entry = rowFromEvent(event)
            if (entry) openEntry(entry)
          }}
          onContextMenu={(event) => {
            const entry = rowFromEvent(event)
            if (!entry) return
            event.preventDefault()
            setSelected(entry.name)
            setMenuFor(entry.name)
          }}
          onDragOver={(event) => {
            if (!event.dataTransfer.types.includes("Files")) return
            event.preventDefault()
            setDragging(true)
          }}
          onDragLeave={(event) => {
            const next = event.relatedTarget
            if (!(next instanceof Node) || !event.currentTarget.contains(next)) setDragging(false)
          }}
          onDrop={onDrop}
        >
          {creating && (
            <NameField
              initial=""
              label={creating === "folder" ? "New folder name" : "New file name"}
              icon={
                creating === "folder" ? (
                  <FolderIcon className="size-4 text-sky-500" />
                ) : (
                  <FileIcon className="size-4" />
                )
              }
              onSubmit={(name) => void create(creating, name)}
              onCancel={() => setCreating(null)}
            />
          )}
          {folder.isPending ? (
            <div className="flex flex-col gap-2 p-3">
              <Skeleton className="h-6 w-3/4" />
              <Skeleton className="h-6 w-1/2" />
              <Skeleton className="h-6 w-2/3" />
            </div>
          ) : folder.error ? (
            <div className="flex flex-col items-center gap-3 p-6 text-center text-sm text-muted-foreground">
              <p>{folderError(folder.error)}</p>
              <Button size="sm" variant="outline" onClick={() => navigate(SANDBOX_HOME)}>
                Go to {SANDBOX_HOME}
              </Button>
            </div>
          ) : entries.length === 0 && !creating ? (
            <p className="p-6 text-center text-sm text-muted-foreground">
              {folder.data.entries.length > 0
                ? "Only hidden files here."
                : "This folder is empty. Drop files here to upload them."}
            </p>
          ) : (
            entries.map((entry, index) =>
              renaming === entry.name ? (
                <NameField
                  key={entry.name}
                  initial={entry.name}
                  label={`Rename ${entry.name}`}
                  icon={<EntryIcon entry={entry} />}
                  onSubmit={(name) => rename(entry, name)}
                  onCancel={() => {
                    setRenaming(null)
                    listRef.current?.focus()
                  }}
                />
              ) : (
                <div
                  key={entry.name}
                  id={rowId(index)}
                  role="option"
                  aria-selected={entry.name === selected}
                  data-entry={entry.name}
                  className={cn(
                    "group/row mx-1 flex h-9 cursor-default items-center gap-3 rounded-md pr-1 pl-2 text-sm",
                    entry.name === selected ? "bg-muted" : "hover:bg-muted/50",
                    open?.path === joinPath(path, entry.name) && "font-medium",
                  )}
                >
                  <EntryIcon entry={entry} />
                  <span className="min-w-0 flex-1 truncate">{entry.name}</span>
                  <span className="hidden w-20 shrink-0 text-right text-xs text-muted-foreground tabular-nums sm:block">
                    {isFolder(entry) ? "—" : formatBytes(entry.size)}
                  </span>
                  <Tooltip>
                    <TooltipTrigger
                      render={<span />}
                      className="hidden w-28 shrink-0 truncate text-right text-xs text-muted-foreground md:block"
                    >
                      {timeAgo(entry.modifiedAt, now)}
                    </TooltipTrigger>
                    <TooltipContent>{fullDate.format(new Date(entry.modifiedAt))}</TooltipContent>
                  </Tooltip>
                  <DropdownMenu
                    open={menuFor === entry.name}
                    onOpenChange={(next) => setMenuFor(next ? entry.name : null)}
                  >
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          tabIndex={-1}
                          aria-label={`More for ${entry.name}`}
                          className="opacity-100 group-hover/row:opacity-100 group-aria-selected/row:opacity-100 data-popup-open:opacity-100 md:opacity-0"
                        />
                      }
                    >
                      <EllipsisIcon />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-44" finalFocus={listRef}>
                      <DropdownMenuItem onClick={() => openEntry(entry)}>Open</DropdownMenuItem>
                      {!isFolder(entry) && (
                        <DropdownMenuItem
                          onClick={() =>
                            downloadFile(fileUrl(agentId, joinPath(path, entry.name)), entry.name)
                          }
                        >
                          Download
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem onClick={() => setRenaming(entry.name)}>
                        Rename
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={() => {
                          move.reset()
                          setMoving(joinPath(path, entry.name))
                        }}
                      >
                        Move…
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => setDeleting(entry)}>
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              ),
            )
          )}
        </div>
      </div>

      {open && (
        <FileViewer
          key={open.path}
          agentId={agentId}
          path={open.path}
          entry={open.entry}
          mobile={mobile}
          confirmingDiscard={pendingNav !== null}
          onDirtyChange={setDirty}
          onSaved={() => void refresh(path)}
          onClose={() =>
            guard(() => {
              setOpen(null)
              setDirty(false)
            })
          }
          onKeepEditing={() => setPendingNav(null)}
          onDiscard={() => {
            const action = pendingNav
            setPendingNav(null)
            setDirty(false)
            action?.()
          }}
        />
      )}

      <MoveDialog
        entryPath={moving}
        pending={move.isPending}
        error={move.error?.message}
        onClose={() => setMoving(null)}
        onMove={(to) =>
          move.mutate(
            { from: moving ?? "", to },
            {
              onSuccess: () => {
                setMoving(null)
                toast.success(`Moved to ${to}`)
              },
            },
          )
        }
      />
      <DeleteDialog
        entry={deleting}
        pending={remove.isPending}
        onClose={() => setDeleting(null)}
        onDelete={() => {
          if (!deleting) return
          const target = joinPath(path, deleting.name)
          remove.mutate(target, {
            onSuccess: () => {
              setDeleting(null)
              if (open && (open.path === target || open.path.startsWith(`${target}/`)))
                setOpen(null)
              listRef.current?.focus()
            },
            onError: (error) => toast.error(`Couldn't delete ${deleting.name}: ${error.message}`),
          })
        }}
      />
    </div>
  )
}

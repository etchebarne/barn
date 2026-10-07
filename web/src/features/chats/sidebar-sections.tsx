import {
  closestCenter,
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useDroppable,
  useSensor,
  useSensors,
  type Announcements,
  type CollisionDetection,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core"
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable"
import { CSS } from "@dnd-kit/utilities"
import { Link } from "@tanstack/react-router"
import { cn } from "cn"
import { ChevronRightIcon, EllipsisIcon, FolderInputIcon, PlusIcon } from "lucide-react"
import { useState, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"
import { AgentAvatar, GroupAvatar } from "@/features/agents"
import type { Agent, Chat } from "@/lib/api-client"
import { readStorage, writeStorage } from "@/lib/storage"

import { usePaletteStore } from "./palette-store"
import { chatPreview, dmAgent } from "./preview"
import {
  useCreateCategory,
  useDeleteCategory,
  useSaveLayout,
  useUpdateCategory,
} from "./sidebar-api"
import {
  buildSections,
  findSection,
  layoutPayload,
  moveCategory,
  moveChat,
  sectionKey,
  sectionUnread,
  UNASSIGNED,
  visibleSections,
  type Section,
  type SectionKey,
  type SidebarCategory,
} from "./sidebar-layout"

type DragData = { type: "chat" } | { type: "category" } | { type: "section" }

function dataOf(item: { data: { current?: unknown } } | null | undefined): DragData | undefined {
  const data = item?.data.current
  if (data && typeof data === "object" && "type" in data) {
    const type = data.type
    if (type === "chat" || type === "category" || type === "section") return { type }
  }
  return undefined
}

function UnreadBadge({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <span
      className="ml-auto flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-medium text-primary-foreground tabular-nums"
      aria-label={`${count} unread`}
    >
      {count > 99 ? "99+" : count}
    </span>
  )
}

function ChatAvatar({ chat, agent }: { chat: Chat; agent: Agent | undefined }) {
  if (chat.kind === "dm") {
    return (
      <AgentAvatar
        id={agent?.id}
        name={agent?.name ?? chat.name}
        active={agent?.activity.state === "working"}
      />
    )
  }
  return <GroupAvatar memberIds={chat.members.map((m) => m.agentId)} />
}

/** A chat row's content: avatar, name, last message, unread badge. Also the drag overlay. */
function ChatRowContent({ chat, agents }: { chat: Chat; agents: Map<string, Agent> }) {
  const unread = chat.unreadCount > 0
  return (
    <>
      <ChatAvatar chat={chat} agent={dmAgent(chat, agents)} />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate font-medium">{chat.name}</span>
        <span
          className={cn(
            "truncate text-xs",
            unread ? "text-sidebar-foreground" : "text-muted-foreground",
          )}
        >
          {chatPreview(chat, agents)}
        </span>
      </div>
      <UnreadBadge count={chat.unreadCount} />
    </>
  )
}

/** "Move to →": a keyboard and touch friendly alternative to dragging. */
function MoveToMenu({
  chat,
  sections,
  onMove,
}: {
  chat: Chat
  sections: Section[]
  onMove: (toKey: SectionKey) => void
}) {
  const current = findSection(sections, chat.id)?.key
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`More for ${chat.name}`}
            className="absolute top-1/2 right-2 -translate-y-1/2 opacity-100 group-focus-within/menu-item:opacity-100 group-hover/menu-item:opacity-100 data-popup-open:opacity-100 md:opacity-0 [@media(hover:none)]:opacity-100"
          />
        }
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <FolderInputIcon />
            Move to
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {sections.map((section) => (
              <DropdownMenuItem
                key={section.key}
                disabled={section.key === current}
                onClick={() => onMove(section.key)}
              >
                {section.name}
              </DropdownMenuItem>
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function SortableChatRow({
  chat,
  agents,
  active,
  indicator,
  menu,
  onNavigate,
}: {
  chat: Chat
  agents: Map<string, Agent>
  active: boolean
  /** A chat from another section would be dropped right above this row. */
  indicator: boolean
  menu: ReactNode
  onNavigate: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: chat.id,
    data: { type: "chat" },
  })
  // The link keeps its own role and focus; it gets the sortable's description and listeners.
  const { role: _role, tabIndex: _tabIndex, ...a11y } = attributes
  return (
    <SidebarMenuItem
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={cn(
        "relative",
        isDragging && "opacity-40",
        indicator &&
          "before:absolute before:inset-x-2 before:-top-px before:h-0.5 before:rounded-full before:bg-primary",
      )}
    >
      <SidebarMenuButton
        size="lg"
        isActive={active}
        className={cn("h-auto gap-3 py-2 select-none", menu !== null && "pr-8")}
        render={<Link to="/chats/$chatId" params={{ chatId: chat.id }} onClick={onNavigate} />}
        {...a11y}
        {...listeners}
      >
        <ChatRowContent chat={chat} agents={agents} />
      </SidebarMenuButton>
      {menu}
    </SidebarMenuItem>
  )
}

function InlineName({
  initial,
  label,
  onSave,
  onCancel,
}: {
  initial: string
  label: string
  onSave: (name: string) => void
  onCancel: () => void
}) {
  const [value, setValue] = useState(initial)
  function save() {
    const name = value.trim()
    if (!name || name === initial) onCancel()
    else onSave(name.slice(0, 40))
  }
  return (
    <Input
      aria-label={label}
      autoFocus
      maxLength={40}
      value={value}
      className="h-7 px-2 text-xs"
      onChange={(e) => setValue(e.target.value)}
      onBlur={save}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault()
          save()
        } else if (e.key === "Escape") {
          // Cancel the rename without closing the (mobile) sidebar sheet.
          e.preventDefault()
          e.stopPropagation()
          onCancel()
        }
      }}
    />
  )
}

/** A section's header: name, collapse, unread sum when collapsed, and Rename/Delete. */
function SectionHeader({
  section,
  dropTarget,
  dragHandle,
  onToggle,
  onRename,
  onDelete,
}: {
  section: Section
  dropTarget: boolean
  dragHandle: { ref: (el: HTMLElement | null) => void; props: Record<string, unknown> } | null
  onToggle: () => void
  onRename: (name: string) => void
  onDelete: () => void
}) {
  const [renaming, setRenaming] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const unread = section.collapsed ? sectionUnread(section) : 0
  const isCategory = section.categoryId !== null

  return (
    <div className="flex flex-col gap-1">
      <div
        ref={dragHandle?.ref}
        {...(dragHandle?.props ?? {})}
        className={cn(
          // Same grid as chat rows: an 8 (2rem) icon column where avatars sit, then the label
          // where chat names start, and actions on the same right edge as the rows' ⋯.
          "group/section flex h-8 items-center gap-3 rounded-md px-2 text-xs font-medium text-muted-foreground outline-none select-none focus-visible:ring-2 focus-visible:ring-sidebar-ring",
          dropTarget && "bg-sidebar-accent ring-1 ring-primary/40",
        )}
      >
        <span className="flex w-8 shrink-0 justify-center">
          <button
            type="button"
            aria-expanded={!section.collapsed}
            aria-label={`${section.collapsed ? "Expand" : "Collapse"} ${section.name}`}
            className="flex size-6 items-center justify-center rounded outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            onClick={onToggle}
          >
            <ChevronRightIcon
              aria-hidden="true"
              className={cn("size-3.5", !section.collapsed && "rotate-90")}
            />
          </button>
        </span>
        {renaming ? (
          <InlineName
            initial={section.name}
            label="Category name"
            onSave={(name) => {
              setRenaming(false)
              onRename(name)
            }}
            onCancel={() => setRenaming(false)}
          />
        ) : (
          <span className="min-w-0 flex-1 truncate">{section.name}</span>
        )}
        {unread > 0 && !renaming && (
          <span
            className="flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] text-primary-foreground tabular-nums"
            aria-label={`${unread} unread in ${section.name}`}
          >
            {unread > 99 ? "99+" : unread}
          </span>
        )}
        {isCategory && !renaming && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`${section.name} options`}
                  className="opacity-100 group-focus-within/section:opacity-100 group-hover/section:opacity-100 data-popup-open:opacity-100 md:opacity-0 [@media(hover:none)]:opacity-100"
                />
              }
            >
              <EllipsisIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40">
              <DropdownMenuItem onClick={() => setRenaming(true)}>Rename</DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={() => setConfirming(true)}>
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      {confirming && (
        <div
          role="alertdialog"
          aria-label={`Delete ${section.name}?`}
          className="mx-1 flex flex-col gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-2 text-xs"
        >
          <p>Delete {section.name}? Its chats move to Unassigned.</p>
          <div className="flex justify-end gap-1.5">
            <Button size="xs" variant="ghost" autoFocus onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button
              size="xs"
              variant="destructive"
              onClick={() => {
                setConfirming(false)
                onDelete()
              }}
            >
              Delete
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

function CategoryHeader(props: Omit<Parameters<typeof SectionHeader>[0], "dragHandle">) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: props.section.key, data: { type: "category" } })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={cn(isDragging && "opacity-40")}
    >
      <SectionHeader
        {...props}
        dragHandle={{
          ref: setActivatorNodeRef,
          props: { ...attributes, ...listeners, "aria-roledescription": "sortable category" },
        }}
      />
    </div>
  )
}

function UnassignedHeader(props: Omit<Parameters<typeof SectionHeader>[0], "dragHandle">) {
  const { setNodeRef } = useDroppable({ id: UNASSIGNED, data: { type: "section" } })
  return (
    <div ref={setNodeRef}>
      <SectionHeader {...props} dragHandle={null} />
    </div>
  )
}

/** Categories only collide with category headers; chats with chats and section headers. */
const collisionDetection: CollisionDetection = (args) =>
  closestCenter({
    ...args,
    droppableContainers: args.droppableContainers.filter((container) =>
      dataOf(args.active)?.type === "category" ? dataOf(container)?.type === "category" : true,
    ),
  })

const UNASSIGNED_COLLAPSED_KEY = "sidebar:unassigned-collapsed"

/** Where a dragged chat would land, for the drop indicator. */
type DropTarget = { key: SectionKey; beforeChatId: string | null } | null

/**
 * The chat list, arranged by the user: categories with collapsible headers (Unassigned last),
 * drag and drop for chats and categories (pointer, long-press touch, keyboard), and "Move to".
 * Without categories it's one plain list, still reorderable.
 */
export function SidebarSections({
  chats,
  categories,
  agents,
  isActive,
  onNavigate,
}: {
  chats: Chat[]
  categories: SidebarCategory[]
  agents: Map<string, Agent>
  isActive: (chatId: string) => boolean
  onNavigate: () => void
}) {
  const save = useSaveLayout()
  const update = useUpdateCategory()
  const remove = useDeleteCategory()
  const [activeId, setActiveId] = useState<UniqueIdentifier | null>(null)
  const [activeType, setActiveType] = useState<DragData["type"] | null>(null)
  const [dropTarget, setDropTarget] = useState<DropTarget>(null)

  // Unassigned has no server-side category, so its collapsed state is kept on this device.
  const [unassignedCollapsed, setUnassignedCollapsed] = useState(
    () => readStorage(UNASSIGNED_COLLAPSED_KEY) === "1",
  )
  const sections = buildSections(chats, categories).map((section) =>
    section.key === UNASSIGNED
      ? Object.assign({}, section, { collapsed: unassignedCollapsed })
      : section,
  )
  const hasCategories = categories.length > 0
  const shown = visibleSections(sections, activeType === "chat")
  const categoryOrder = categories.map((c) => c.id)
  const categoryKeys = categories.map((c) => sectionKey(c.id))

  const sensors = useSensors(
    // A small distance so a click on a chat is never a drag.
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    // Long-press on touch, so scrolling the list still works.
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
      // Space picks up and drops; Enter on a chat link still opens it.
      keyboardCodes: { start: ["Space"], cancel: ["Escape"], end: ["Space", "Enter"] },
    }),
  )

  function labelOf(id: UniqueIdentifier | undefined): string {
    if (id === undefined) return "nothing"
    const key = String(id)
    const section = sections.find((s) => s.key === key)
    if (section) return `the ${section.name} section`
    return chats.find((c) => c.id === key)?.name ?? "item"
  }

  const announcements: Announcements = {
    onDragStart: ({ active }) => `Picked up ${labelOf(active.id)}.`,
    onDragOver: ({ active, over }) =>
      over
        ? `${labelOf(active.id)} is over ${labelOf(over.id)}.`
        : `${labelOf(active.id)} is no longer over a drop area.`,
    onDragEnd: ({ active, over }) =>
      over
        ? `${labelOf(active.id)} was dropped on ${labelOf(over.id)}.`
        : `${labelOf(active.id)} was dropped.`,
    onDragCancel: ({ active }) => `Moving ${labelOf(active.id)} was cancelled.`,
  }

  /** The section and index a chat would be dropped at, given what it's over. */
  function chatTarget(overId: UniqueIdentifier, overType: DragData["type"] | undefined) {
    const id = String(overId)
    if (overType === "chat") {
      const section = findSection(sections, id)
      if (!section) return null
      return {
        key: section.key,
        index: section.chats.findIndex((c) => c.id === id),
        beforeChatId: id,
      }
    }
    const section = sections.find((s) => s.key === id)
    if (!section) return null
    return { key: section.key, index: section.chats.length, beforeChatId: null }
  }

  function moveTo(chatId: string, toKey: SectionKey, toIndex: number) {
    const result = moveChat(sections, chatId, toKey, toIndex)
    if (!result) return
    save.mutate(layoutPayload(categoryOrder, result.sections, result.changed))
  }

  function onDragStart(event: DragStartEvent) {
    setActiveId(event.active.id)
    setActiveType(dataOf(event.active)?.type ?? null)
  }

  function onDragOver(event: DragOverEvent) {
    if (dataOf(event.active)?.type !== "chat" || !event.over) {
      setDropTarget(null)
      return
    }
    const target = chatTarget(event.over.id, dataOf(event.over)?.type)
    const from = findSection(sections, String(event.active.id))
    // Within a section the rows shift to show the spot; across sections, draw a line.
    setDropTarget(
      target && target.key !== from?.key
        ? { key: target.key, beforeChatId: target.beforeChatId }
        : null,
    )
  }

  function reset() {
    setActiveId(null)
    setActiveType(null)
    setDropTarget(null)
  }

  function onDragEnd(event: DragEndEvent) {
    const { active, over } = event
    const type = dataOf(active)?.type
    reset()
    if (!over) return
    if (type === "chat") {
      const target = chatTarget(over.id, dataOf(over)?.type)
      if (target) moveTo(String(active.id), target.key, target.index)
      return
    }
    if (type === "category") {
      const from = categoryKeys.indexOf(String(active.id))
      const to = categoryKeys.indexOf(String(over.id))
      if (from === -1 || to === -1 || from === to) return
      const order = moveCategory(categoryOrder, from, to)
      save.mutate(layoutPayload(order, sections, []))
    }
  }

  const activeChat = activeType === "chat" ? chats.find((c) => c.id === activeId) : undefined
  const activeSection =
    activeType === "category" ? sections.find((s) => s.key === activeId) : undefined

  function rows(section: Section) {
    return (
      <SortableContext
        items={section.chats.map((c) => c.id)}
        strategy={verticalListSortingStrategy}
      >
        <SidebarMenu aria-label={hasCategories ? section.name : "Chats"} className="gap-0.5">
          {section.chats.map((chat) => (
            <SortableChatRow
              key={chat.id}
              chat={chat}
              agents={agents}
              active={isActive(chat.id)}
              indicator={dropTarget?.key === section.key && dropTarget.beforeChatId === chat.id}
              onNavigate={onNavigate}
              menu={
                hasCategories ? (
                  <MoveToMenu
                    chat={chat}
                    sections={sections}
                    onMove={(toKey) => {
                      const dest = sections.find((s) => s.key === toKey)
                      moveTo(chat.id, toKey, dest?.chats.length ?? 0)
                    }}
                  />
                ) : null
              }
            />
          ))}
        </SidebarMenu>
      </SortableContext>
    )
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collisionDetection}
      accessibility={{
        announcements,
        screenReaderInstructions: {
          draggable:
            "To pick up a chat or category, press Space. Use the arrow keys to move it, Space to drop it, or Escape to cancel.",
        },
      }}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragEnd={onDragEnd}
      onDragCancel={reset}
    >
      {hasCategories ? (
        <div className="flex flex-col gap-3">
          <SortableContext items={categoryKeys} strategy={verticalListSortingStrategy}>
            {shown.map((section) => {
              const headerProps = {
                section,
                dropTarget: dropTarget?.key === section.key && dropTarget.beforeChatId === null,
                onToggle: () => {
                  if (section.categoryId) {
                    update.mutate({ id: section.categoryId, collapsed: !section.collapsed })
                  } else {
                    writeStorage(UNASSIGNED_COLLAPSED_KEY, unassignedCollapsed ? "0" : "1")
                    setUnassignedCollapsed(!unassignedCollapsed)
                  }
                },
                onRename: (name: string) =>
                  section.categoryId && update.mutate({ id: section.categoryId, name }),
                onDelete: () => section.categoryId && remove.mutate(section.categoryId),
              }
              return (
                <section
                  key={section.key}
                  aria-label={section.name}
                  className="flex flex-col gap-0.5"
                >
                  {section.categoryId === null ? (
                    <UnassignedHeader {...headerProps} />
                  ) : (
                    <CategoryHeader {...headerProps} />
                  )}
                  {!section.collapsed && rows(section)}
                </section>
              )
            })}
          </SortableContext>
        </div>
      ) : (
        rows(sections.find((s) => s.key === UNASSIGNED) ?? sections[sections.length - 1])
      )}
      <DragOverlay dropAnimation={{ duration: 150, easing: "cubic-bezier(0.23, 1, 0.32, 1)" }}>
        {activeChat ? (
          <div className="flex items-center gap-3 rounded-md bg-sidebar p-2 text-sm shadow-lg ring-1 ring-sidebar-border">
            <ChatRowContent chat={activeChat} agents={agents} />
          </div>
        ) : activeSection ? (
          <div className="flex h-7 items-center rounded-md bg-sidebar px-3 text-xs font-medium text-muted-foreground shadow-lg ring-1 ring-sidebar-border">
            {activeSection.name}
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  )
}

/** "+ New category": type a name inline; Enter creates it, Escape cancels. */
export function NewCategory() {
  const create = useCreateCategory()
  const [naming, setNaming] = useState(false)
  // The command palette's "New category" starts naming here.
  const request = usePaletteStore((s) => s.newCategoryRequest)
  const [handled, setHandled] = useState(request)
  if (request !== handled) {
    setHandled(request)
    setNaming(true)
  }
  if (naming) {
    return (
      <div className="py-1 pr-2 pl-[3.25rem]">
        <InlineName
          initial=""
          label="New category name"
          onSave={(name) => {
            setNaming(false)
            create.mutate(name)
          }}
          onCancel={() => setNaming(false)}
        />
      </div>
    )
  }
  return (
    <button
      type="button"
      className="flex h-8 w-full items-center gap-3 rounded-md px-2 text-xs text-muted-foreground outline-none select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring"
      onClick={() => setNaming(true)}
    >
      <span className="flex w-8 shrink-0 justify-center" aria-hidden="true">
        <PlusIcon className="size-3.5" />
      </span>
      New category
    </button>
  )
}

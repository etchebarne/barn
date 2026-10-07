import type { Chat, Schemas } from "@/lib/api-client"

export type SidebarCategory = Schemas["SidebarCategory"]
export type SidebarLayout = Schemas["SidebarLayout"]

/** "cat:<id>" for categories, "unassigned" for chats without one. Used as drag ids too. */
export type SectionKey = string
export const UNASSIGNED: SectionKey = "unassigned"

export function sectionKey(categoryId: string | null): SectionKey {
  return categoryId === null ? UNASSIGNED : `cat:${categoryId}`
}

export function categoryIdOf(key: SectionKey): string | null {
  return key === UNASSIGNED ? null : key.slice("cat:".length)
}

export type Section = {
  key: SectionKey
  categoryId: string | null
  name: string
  collapsed: boolean
  chats: Chat[]
}

/**
 * Orders a section's chats: chats never placed by the user (null position) first, in the
 * list's order (most recently active first), then placed chats by position.
 */
export function orderChats(chats: Chat[]): Chat[] {
  const unplaced = chats.filter((c) => c.position === null)
  const placed = chats
    .filter((c) => c.position !== null)
    .toSorted((a, b) => (a.position ?? 0) - (b.position ?? 0))
  return [...unplaced, ...placed]
}

/**
 * Groups chats into sections: one per category, in order, then "Unassigned" last. Chats in a
 * category that no longer exists count as unassigned. `chats` is the server's list (by activity).
 */
export function buildSections(chats: Chat[], categories: SidebarCategory[]): Section[] {
  const known = new Set(categories.map((c) => c.id))
  const byCategory = new Map<string | null, Chat[]>()
  for (const chat of chats) {
    const id = chat.categoryId !== null && known.has(chat.categoryId) ? chat.categoryId : null
    byCategory.set(id, [...(byCategory.get(id) ?? []), chat])
  }
  return [
    ...categories.map((category) => ({
      key: sectionKey(category.id),
      categoryId: category.id,
      name: category.name,
      collapsed: category.collapsed,
      chats: orderChats(byCategory.get(category.id) ?? []),
    })),
    {
      key: UNASSIGNED,
      categoryId: null,
      name: "Unassigned",
      collapsed: false,
      chats: orderChats(byCategory.get(null) ?? []),
    },
  ]
}

/** Sections to render: "Unassigned" only when it has chats, or while dragging (as a target). */
export function visibleSections(sections: Section[], dragging: boolean): Section[] {
  return sections.filter((s) => s.key !== UNASSIGNED || dragging || s.chats.length > 0)
}

/** Unread messages in a section, shown on its header when collapsed. */
export function sectionUnread(section: Section): number {
  return section.chats.reduce((sum, chat) => sum + chat.unreadCount, 0)
}

export function findSection(sections: Section[], chatId: string): Section | undefined {
  return sections.find((s) => s.chats.some((c) => c.id === chatId))
}

/**
 * Moves a chat to `toKey` at `toIndex` (an index in the destination's list without the moved
 * chat; clamped). Returns the new sections and the keys of the sections that changed (source
 * and destination, or one when moving within a section). Null when nothing changes.
 */
export function moveChat(
  sections: Section[],
  chatId: string,
  toKey: SectionKey,
  toIndex: number,
): { sections: Section[]; changed: SectionKey[] } | null {
  const from = findSection(sections, chatId)
  const to = sections.find((s) => s.key === toKey)
  if (!from || !to) return null
  const chat = from.chats.find((c) => c.id === chatId)
  if (!chat) return null
  const fromIndex = from.chats.indexOf(chat)
  const remaining = (to.key === from.key ? from.chats : to.chats).filter((c) => c.id !== chatId)
  const index = Math.max(0, Math.min(toIndex, remaining.length))
  if (to.key === from.key && index === fromIndex) return null
  const inserted = [...remaining.slice(0, index), chat, ...remaining.slice(index)]
  const next = sections.map((s) => {
    if (s.key === to.key) return { ...s, chats: inserted }
    if (s.key === from.key) return { ...s, chats: from.chats.filter((c) => c.id !== chatId) }
    return s
  })
  const changed = from.key === to.key ? [to.key] : [from.key, to.key]
  return { sections: next, changed }
}

/** Moves a category from one index to another. */
export function moveCategory(order: string[], fromIndex: number, toIndex: number): string[] {
  if (fromIndex === toIndex) return order
  const next = [...order]
  const [moved] = next.splice(fromIndex, 1)
  if (moved === undefined) return order
  next.splice(Math.max(0, Math.min(toIndex, next.length)), 0, moved)
  return next
}

/** The PUT /sidebar/layout body: every category in order, and each changed section in full. */
export function layoutPayload(
  categoryOrder: string[],
  sections: Section[],
  changed: SectionKey[],
): SidebarLayout {
  return {
    categoryOrder,
    sections: changed.flatMap((key) => {
      const section = sections.find((s) => s.key === key)
      return section
        ? [{ categoryId: section.categoryId, chatIds: section.chats.map((c) => c.id) }]
        : []
    }),
  }
}

/** The chat list as the server will have it after a layout PUT (for an optimistic update). */
export function applyLayoutToChats(chats: Chat[], layout: SidebarLayout): Chat[] {
  const placement = new Map<string, { categoryId: string | null; position: number }>()
  for (const section of layout.sections) {
    section.chatIds.forEach((id, position) =>
      placement.set(id, { categoryId: section.categoryId, position }),
    )
  }
  return chats.map((chat) => {
    const place = placement.get(chat.id)
    return place ? Object.assign({}, chat, place) : chat
  })
}

/** Categories reordered to match `order`. */
export function orderCategories(categories: SidebarCategory[], order: string[]): SidebarCategory[] {
  const byId = new Map(categories.map((c) => [c.id, c]))
  return order.flatMap((id) => {
    const category = byId.get(id)
    return category ? [category] : []
  })
}

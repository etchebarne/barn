import { describe, expect, it } from "vitest"

import type { Chat } from "@/lib/api-client"
import { makeChat } from "@/test/fixtures"

import {
  applyLayoutToChats,
  buildSections,
  layoutPayload,
  moveCategory,
  moveChat,
  orderCategories,
  orderChats,
  sectionUnread,
  UNASSIGNED,
  visibleSections,
  type SidebarCategory,
} from "./sidebar-layout"

const chat = (id: string, categoryId: string | null, position: number | null, unread = 0): Chat =>
  makeChat({ id, name: id, categoryId, position, unreadCount: unread })

const casino: SidebarCategory = { id: "casino", name: "casino", collapsed: false }
const groups: SidebarCategory = { id: "groups", name: "groups", collapsed: true }

// The server's list order is by recent activity.
const chats = [
  chat("grok", null, null, 2),
  chat("ona", "casino", 1),
  chat("fresh", "casino", null),
  chat("claude", "casino", 0, 1),
  chat("team", "groups", 0, 3),
  chat("lost", "gone-category", 4),
]

const ids = (list: Chat[]) => list.map((c) => c.id)

describe("ordering", () => {
  it("puts never-placed chats first by activity, then placed ones by position", () => {
    expect(
      ids(
        orderChats([
          chat("b", null, 1),
          chat("x", null, null),
          chat("a", null, 0),
          chat("y", null, null),
        ]),
      ),
    ).toEqual(["x", "y", "a", "b"])
  })

  it("groups chats into category sections then Unassigned last", () => {
    const sections = buildSections(chats, [casino, groups])
    expect(sections.map((s) => [s.key, s.name, ids(s.chats)])).toEqual([
      ["cat:casino", "casino", ["fresh", "claude", "ona"]],
      ["cat:groups", "groups", ["team"]],
      // A chat in a category that no longer exists counts as unassigned.
      [UNASSIGNED, "Unassigned", ["grok", "lost"]],
    ])
  })

  it("hides an empty Unassigned unless dragging", () => {
    const sections = buildSections([chat("a", "casino", 0)], [casino])
    expect(visibleSections(sections, false).map((s) => s.key)).toEqual(["cat:casino"])
    expect(visibleSections(sections, true).map((s) => s.key)).toEqual(["cat:casino", UNASSIGNED])
  })

  it("sums unread per section", () => {
    const sections = buildSections(chats, [casino, groups])
    expect(sections.map(sectionUnread)).toEqual([1, 3, 2])
  })
})

describe("moving chats", () => {
  const sections = buildSections(chats, [casino, groups])

  it("reorders within a section and sends only that section", () => {
    const result = moveChat(sections, "ona", "cat:casino", 0)
    expect(result?.changed).toEqual(["cat:casino"])
    expect(layoutPayload(["casino", "groups"], result!.sections, result!.changed)).toEqual({
      categoryOrder: ["casino", "groups"],
      sections: [{ categoryId: "casino", chatIds: ["ona", "fresh", "claude"] }],
    })
  })

  it("moves between sections and sends both, source first", () => {
    const result = moveChat(sections, "grok", "cat:casino", 1)
    expect(layoutPayload(["casino", "groups"], result!.sections, result!.changed)).toEqual({
      categoryOrder: ["casino", "groups"],
      sections: [
        { categoryId: null, chatIds: ["lost"] },
        { categoryId: "casino", chatIds: ["fresh", "grok", "claude", "ona"] },
      ],
    })
  })

  it("appends when dropped on a header or past the end, and ignores no-op moves", () => {
    const result = moveChat(sections, "team", UNASSIGNED, 99)
    expect(ids(result!.sections.find((s) => s.key === UNASSIGNED)!.chats)).toEqual([
      "grok",
      "lost",
      "team",
    ])
    expect(ids(result!.sections.find((s) => s.key === "cat:groups")!.chats)).toEqual([])
    expect(moveChat(sections, "fresh", "cat:casino", 0)).toBeNull()
    expect(moveChat(sections, "nope", "cat:casino", 0)).toBeNull()
  })

  it("applies a layout to the chat list optimistically", () => {
    const layout = layoutPayload(
      ["casino"],
      moveChat(sections, "grok", "cat:casino", 0)!.sections,
      [UNASSIGNED, "cat:casino"],
    )
    const next = applyLayoutToChats(chats, layout)
    expect(next.find((c) => c.id === "grok")).toMatchObject({ categoryId: "casino", position: 0 })
    expect(next.find((c) => c.id === "lost")).toMatchObject({ categoryId: null, position: 0 })
    expect(next.find((c) => c.id === "ona")).toMatchObject({ categoryId: "casino", position: 3 })
  })
})

describe("moving categories", () => {
  it("reorders and keeps every id", () => {
    expect(moveCategory(["a", "b", "c"], 0, 2)).toEqual(["b", "c", "a"])
    expect(moveCategory(["a", "b", "c"], 2, 0)).toEqual(["c", "a", "b"])
    expect(orderCategories([casino, groups], ["groups", "casino"]).map((c) => c.id)).toEqual([
      "groups",
      "casino",
    ])
  })
})

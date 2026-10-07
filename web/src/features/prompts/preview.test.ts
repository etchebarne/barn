import { describe, expect, it } from "vitest"

import type { Prompt } from "./logic"
import {
  formatPrimitive,
  humanizeKey,
  isLongText,
  looksLikeCode,
  previewStatus,
  toPreviewNode,
} from "./preview"

describe("humanizeKey", () => {
  it("turns snake_case and camelCase into sentence case", () => {
    expect(humanizeKey("thread_ts")).toBe("Thread ts")
    expect(humanizeKey("dueDate")).toBe("Due date")
    expect(humanizeKey("assignee-id")).toBe("Assignee id")
    expect(humanizeKey("URL")).toBe("URL")
  })
})

describe("toPreviewNode", () => {
  it("shows strings as text, keeping line breaks and spotting ids", () => {
    expect(toPreviewNode("hello")).toEqual({
      kind: "text",
      text: "hello",
      multiline: false,
      mono: false,
    })
    expect(toPreviewNode("a\nb")).toMatchObject({ kind: "text", multiline: true })
    expect(looksLikeCode("C0123ABCDEFGHIJKLMNOPQ")).toBe(true)
    expect(toPreviewNode("C0123ABCDEFGHIJKLMNOPQ")).toMatchObject({ mono: true })
    expect(looksLikeCode("a long sentence with spaces in it")).toBe(false)
  })

  it("shows numbers, booleans and empty values readably", () => {
    expect(toPreviewNode(3)).toMatchObject({ kind: "text", text: "3" })
    expect(toPreviewNode(true)).toMatchObject({ kind: "text", text: "Yes" })
    expect(toPreviewNode(false)).toMatchObject({ kind: "text", text: "No" })
    expect(formatPrimitive(null)).toBe("—")
    expect(toPreviewNode(null)).toEqual({ kind: "empty", text: "—" })
    expect(toPreviewNode([])).toEqual({ kind: "empty", text: "None" })
  })

  it("shows arrays of primitives as chips", () => {
    expect(toPreviewNode(["bug", "p1", 2, true])).toEqual({
      kind: "chips",
      items: ["bug", "p1", "2", "Yes"],
    })
  })

  it("nests objects as rows with humanized keys", () => {
    expect(toPreviewNode({ teamId: "eng", is_private: false })).toEqual({
      kind: "rows",
      rows: [
        { key: "teamId", label: "Team id", node: expect.objectContaining({ text: "eng" }) },
        { key: "is_private", label: "Is private", node: expect.objectContaining({ text: "No" }) },
      ],
    })
  })

  it("stacks arrays of objects as blocks", () => {
    const node = toPreviewNode([{ name: "Ada" }, { name: "Grace" }])
    expect(node.kind).toBe("blocks")
    if (node.kind !== "blocks") return
    expect(node.items.map((rows) => rows[0]?.node)).toEqual([
      expect.objectContaining({ text: "Ada" }),
      expect.objectContaining({ text: "Grace" }),
    ])
  })

  it("falls back to compact JSON past the depth cap and for mixed arrays", () => {
    const node = toPreviewNode({ a: { b: { c: { d: 1 } } } })
    expect(node).toMatchObject({
      kind: "rows",
      rows: [
        {
          node: {
            kind: "rows",
            rows: [{ node: { kind: "rows", rows: [{ node: { kind: "json", text: '{"d":1}' } }] } }],
          },
        },
      ],
    })
    expect(toPreviewNode([1, { a: 2 }])).toEqual({ kind: "json", text: '[1,{"a":2}]' })
  })
})

describe("isLongText", () => {
  it("collapses past a dozen lines or very long text", () => {
    expect(isLongText(Array.from({ length: 13 }, () => "x").join("\n"))).toBe(true)
    expect(isLongText("short")).toBe(false)
    expect(isLongText("x".repeat(1000))).toBe(true)
  })
})

describe("previewStatus", () => {
  const base: Prompt = {
    kind: "approval",
    question: "Send it?",
    options: [{ label: "Approve" }, { label: "Decline" }],
    allowOther: false,
    status: "pending",
    answer: null,
  }
  it("labels each state", () => {
    expect(previewStatus(base)).toEqual({ label: "Needs approval", tone: "warning" })
    expect(previewStatus({ ...base, status: "answered", answer: { selected: [0] } })).toEqual({
      label: "Approved",
      tone: "done",
    })
    expect(previewStatus({ ...base, status: "answered", answer: { selected: [1] } })).toEqual({
      label: "Declined",
      tone: "muted",
    })
    expect(previewStatus({ ...base, status: "dismissed" })).toEqual({
      label: "Dismissed",
      tone: "muted",
    })
  })
})

import { describe, expect, it } from "vitest"

import { makeMessage } from "@/test/fixtures"

import {
  acceptsLetterKeys,
  answerMessage,
  approvalAnswer,
  approvalOutcome,
  buildAnswer,
  chatToOpen,
  dismissMessage,
  letterIndex,
  optionLetter,
  promptRows,
  type Prompt,
} from "./logic"

function prompt(overrides: Partial<Prompt> = {}): Prompt {
  return {
    kind: "single",
    question: "What should I call you?",
    options: [{ label: "Martin" }, { label: "Boss" }, { label: "Captain", opensChatId: "c-9" }],
    allowOther: true,
    status: "pending",
    answer: null,
    ...overrides,
  }
}

describe("answerMessage / dismissMessage", () => {
  it("marks the prompt answered with the answer, leaving the rest of the message alone", () => {
    const message = makeMessage({ body: "What should I call you?", prompt: prompt() })
    const answered = answerMessage(message, { selected: [1] })
    expect(answered.prompt).toMatchObject({ status: "answered", answer: { selected: [1] } })
    expect(answered.body).toBe(message.body)
    expect(message.prompt?.status).toBe("pending")
  })

  it("dismisses, and ignores messages without a prompt", () => {
    expect(dismissMessage(makeMessage({ prompt: prompt() })).prompt?.status).toBe("dismissed")
    const plain = makeMessage()
    expect(answerMessage(plain, { text: "x" })).toBe(plain)
  })
})

describe("buildAnswer", () => {
  it("combines sorted, unique selections with trimmed text, or returns null when empty", () => {
    expect(buildAnswer([2, 0, 2], "  ")).toEqual({ selected: [0, 2] })
    expect(buildAnswer([], " Marty ")).toEqual({ text: "Marty" })
    expect(buildAnswer([1], "also this")).toEqual({ selected: [1], text: "also this" })
    expect(buildAnswer([], "   ")).toBeNull()
  })
})

describe("promptRows", () => {
  it("shows every option while pending", () => {
    expect(promptRows(prompt()).map((r) => r.type === "option" && r.label)).toEqual([
      "Martin",
      "Boss",
      "Captain",
    ])
  })

  it("collapses to the chosen options plus a typed answer once answered", () => {
    const rows = promptRows(
      prompt({ kind: "multi", status: "answered", answer: { selected: [2, 0], text: "Marty" } }),
    )
    expect(rows).toEqual([
      { type: "option", index: 0, label: "Martin", chosen: true },
      { type: "option", index: 2, label: "Captain", chosen: true },
      { type: "text", text: "Marty" },
    ])
  })

  it("shows nothing for a dismissed prompt", () => {
    expect(promptRows(prompt({ status: "dismissed" }))).toEqual([])
  })
})

function approval(overrides: Partial<Prompt> = {}): Prompt {
  return prompt({
    kind: "approval",
    question: "Delete Tracker?",
    options: [{ label: "Approve" }, { label: "Decline" }],
    allowOther: false,
    ...overrides,
  })
}

describe("approval prompts", () => {
  it("answers Approve with [0] and Decline with [1]", () => {
    expect(approvalAnswer(true)).toEqual({ selected: [0] })
    expect(approvalAnswer(false)).toEqual({ selected: [1] })
  })

  it("reports the outcome once answered", () => {
    expect(approvalOutcome(approval())).toBeNull()
    expect(approvalOutcome(approval({ status: "answered", answer: { selected: [0] } }))).toBe(
      "approved",
    )
    expect(approvalOutcome(approval({ status: "answered", answer: { selected: [1] } }))).toBe(
      "declined",
    )
    expect(approvalOutcome(approval({ status: "dismissed" }))).toBeNull()
  })

  it("never takes letter-key shortcuts", () => {
    expect(acceptsLetterKeys(approval(), true)).toBe(false)
  })
})

describe("keyboard and navigation helpers", () => {
  it("maps letters to options within range", () => {
    expect(optionLetter(0)).toBe("A")
    expect(letterIndex("b", 3)).toBe(1)
    expect(letterIndex("C", 3)).toBe(2)
    expect(letterIndex("d", 3)).toBeNull()
    expect(letterIndex("Enter", 3)).toBeNull()
  })

  it("only takes letter keys on the latest pending choice prompt", () => {
    expect(acceptsLetterKeys(prompt(), true)).toBe(true)
    expect(acceptsLetterKeys(prompt(), false)).toBe(false)
    expect(acceptsLetterKeys(prompt({ kind: "text" }), true)).toBe(false)
    expect(acceptsLetterKeys(prompt({ status: "answered" }), true)).toBe(false)
  })

  it("opens the chat linked from a chosen option", () => {
    expect(chatToOpen(prompt(), { selected: [2] })).toBe("c-9")
    expect(chatToOpen(prompt(), { selected: [0] })).toBeNull()
    expect(chatToOpen(prompt(), { text: "hi" })).toBeNull()
  })
})

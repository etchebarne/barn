import { render, screen } from "@testing-library/react"
import ReactMarkdown from "react-markdown"
import { describe, expect, it } from "vitest"

import { MentionText } from "./mention-text"
import { rehypeMentions } from "./rehype-mentions"

const alpha = { id: "a", name: "Alpha" }

function chips(container: HTMLElement) {
  return [...container.querySelectorAll(".mention")].map((el) => el.textContent)
}

describe("mention highlighting", () => {
  it("chips only the mentioned agents in plain text", () => {
    const { container } = render(
      <p>
        <MentionText text="@Alpha ping @Beta, mail alpha@corp.dev" mentions={[alpha]} />
      </p>,
    )
    expect(chips(container)).toEqual(["@Alpha"])
    expect(container.textContent).toBe("@Alpha ping @Beta, mail alpha@corp.dev")
  })

  it("chips mentions in markdown text but never inside code", () => {
    const { container } = render(
      <ReactMarkdown rehypePlugins={[[rehypeMentions, { targets: [alpha] }]]}>
        {"**@alpha** can you run `@Alpha test`?\n\n```\n@Alpha\n```"}
      </ReactMarkdown>,
    )
    expect(chips(container)).toEqual(["@alpha"])
    expect(screen.getByText("@Alpha test").tagName).toBe("CODE")
  })

  it("leaves text alone when nothing is mentioned", () => {
    const { container } = render(
      <ReactMarkdown rehypePlugins={[[rehypeMentions, { targets: [] }]]}>
        {"hi @Alpha"}
      </ReactMarkdown>,
    )
    expect(chips(container)).toEqual([])
  })
})

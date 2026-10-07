import { render } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { AgentAvatar, GroupAvatar } from "./agent-avatar"
import { Aura, auraMotion, marble } from "./aura"

const gray = ["#111111", "#222222", "#333333", "#444444", "#555555"]

/** Markup with React's per-render ids (the blur filter's) normalized away. */
function markup(seed: string) {
  const { container, unmount } = render(<Aura seed={seed} />)
  const html = container.innerHTML.replace(/aura-blur-[^")]+/g, "aura-blur-ID")
  unmount()
  return html
}

function stubMotion(reduce: boolean) {
  const animate = vi.fn<(keyframes: Keyframe[], options: KeyframeAnimationOptions) => unknown>(
    () => ({ cancel: () => {}, pause: () => {}, play: () => {} }),
  )
  Object.defineProperty(HTMLElement.prototype, "animate", { configurable: true, value: animate })
  const original = window.matchMedia
  vi.spyOn(window, "matchMedia").mockImplementation((query) =>
    Object.assign(original(query), { matches: reduce && query.includes("reduce") }),
  )
  return animate
}

afterEach(() => {
  delete (HTMLElement.prototype as Partial<HTMLElement>).animate
})

describe("aura", () => {
  it("keeps boring-avatars' marble layout, so every agent keeps its picture", () => {
    // Values produced by boring-avatars 2.0.4 `<Avatar variant="marble" name="agent-1" />`.
    expect(marble("agent-1", gray)).toMatchObject({
      background: "#111111",
      blobs: [
        { fill: "#222222", transform: "translate(6 6) rotate(30 40 40) scale(1.3)" },
        { fill: "#333333", transform: "translate(-5 5) rotate(-45 40 40) scale(1.3)" },
      ],
    })
  })

  it("renders the same picture and motion for the same seed", () => {
    expect(markup("agent-1")).toBe(markup("agent-1"))
    expect(markup("agent-1")).not.toBe(markup("agent-2"))
    expect(auraMotion("agent-1")).toEqual(auraMotion("agent-1"))
    expect(auraMotion("agent-1")).not.toEqual(auraMotion("agent-2"))
  })

  it("drifts both blob layers with seeded, out-of-phase timing", () => {
    const animate = stubMotion(false)
    render(<Aura seed="agent-1" />)
    const [a, b] = auraMotion("agent-1").blobs
    expect(animate).toHaveBeenCalledTimes(2)
    expect(animate.mock.calls.map(([, options]) => options)).toEqual([
      expect.objectContaining({ duration: a?.duration, delay: a?.delay, iterations: Infinity }),
      expect.objectContaining({ duration: b?.duration, delay: b?.delay, iterations: Infinity }),
    ])
    expect(a?.delay).toBeLessThanOrEqual(0)
  })

  it("stays still when the user prefers reduced motion", () => {
    const animate = stubMotion(true)
    render(<Aura seed="agent-1" />)
    expect(animate).not.toHaveBeenCalled()
  })

  it("marks working agents as active", () => {
    const { container, rerender } = render(<AgentAvatar id="agent-1" name="Ada" />)
    expect(container.querySelector("[data-aura]")).not.toHaveAttribute("data-active")
    rerender(<AgentAvatar id="agent-1" name="Ada" active />)
    expect(container.querySelector("[data-aura]")).toHaveAttribute("data-active")
  })

  it("falls back to initials, and groups show one aura per member", () => {
    const { container, getByText } = render(<AgentAvatar name="Ada Lovelace" />)
    expect(getByText("AL")).toBeInTheDocument()
    expect(container.querySelector("[data-aura]")).toBeNull()

    const group = render(<GroupAvatar memberIds={["a", "b", "c", "d"]} />)
    expect(group.container.querySelectorAll("[data-aura]")).toHaveLength(3)
  })
})

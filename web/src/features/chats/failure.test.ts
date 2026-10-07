import { describe, expect, it } from "vitest"

import { makeAgent, makeMessage } from "@/test/fixtures"

import { failureActions, failureTone, messageFailure, type Failure } from "./failure"

const blocked: Failure = { agentId: "agent-1", reason: "model_blocked", retryable: true }
const idle = makeAgent()
const working = makeAgent({ activity: { state: "working", label: null } })

describe("failureTone", () => {
  it("treats a turn that reached the step limit as paused, not failed", () => {
    expect(failureTone("too_many_steps")).toBe("neutral")
    expect(failureTone("model_blocked")).toBe("warning")
    expect(failureTone("provider_error")).toBe("destructive")
  })
})

describe("failureActions", () => {
  it("offers Retry only on the latest message", () => {
    expect(failureActions(blocked, { isLatest: true, agent: idle }).retry).toBe(true)
    expect(failureActions(blocked, { isLatest: false, agent: idle }).retry).toBe(false)
  })

  it("offers Retry only when the failure is retryable", () => {
    const stuck: Failure = { agentId: "agent-1", reason: "too_many_steps", retryable: false }
    expect(failureActions(stuck, { isLatest: true, agent: idle }).retry).toBe(false)
  })

  it("offers Change model for blocked models and Open settings for key problems", () => {
    expect(failureActions(blocked, { isLatest: true, agent: idle })).toMatchObject({
      changeModel: true,
      openSettings: false,
    })
    for (const reason of ["no_key", "invalid_key"] as const) {
      expect(
        failureActions(
          { agentId: "agent-1", reason, retryable: true },
          { isLatest: true, agent: idle },
        ),
      ).toMatchObject({ changeModel: false, openSettings: true })
    }
  })

  it("offers no actions on older failures", () => {
    expect(failureActions(blocked, { isLatest: false, agent: idle })).toMatchObject({
      retry: false,
      changeModel: false,
      openSettings: false,
    })
    const keyProblem: Failure = { agentId: "agent-1", reason: "invalid_key", retryable: true }
    expect(failureActions(keyProblem, { isLatest: false, agent: idle }).openSettings).toBe(false)
  })

  it("disables actions while the agent is working", () => {
    expect(failureActions(blocked, { isLatest: true, agent: working }).disabled).toBe(true)
    expect(failureActions(blocked, { isLatest: true, agent: idle }).disabled).toBe(false)
  })
})

describe("messageFailure", () => {
  it("only reads failures from system messages", () => {
    const system = makeMessage({ author: { kind: "system", agentId: null }, failure: blocked })
    expect(messageFailure(system)).toEqual(blocked)
    expect(messageFailure(makeMessage({ failure: blocked }))).toBeNull()
    expect(messageFailure(makeMessage({ author: { kind: "system", agentId: null } }))).toBeNull()
  })
})

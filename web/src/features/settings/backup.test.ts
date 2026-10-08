import { describe, expect, it } from "vitest"

import { parseBackup, planRestore, type AgentsBackup } from "./backup"

function agent(name: string): AgentsBackup["agents"][number] {
  return {
    name,
    instructions: "x",
    personality: "",
    model: "m",
    language: "auto",
    notifications: true,
    trustMode: "ask",
    admin: false,
    memories: [],
    tasks: [],
    connections: [],
    approvals: [],
  }
}

const backup: AgentsBackup = {
  format: "openbot-agents",
  version: 1,
  exportedAt: "2026-10-08T12:00:00Z",
  agents: [agent("Ops"), agent("Research"), agent("research")],
}

describe("parseBackup", () => {
  it("accepts a backup and explains other files", () => {
    expect(parseBackup(JSON.stringify(backup))).toEqual(backup)
    expect(parseBackup("not json")).toBe("That file isn't JSON.")
    expect(parseBackup('{"agents": []}')).toBe("That file isn't an openbot agents backup.")
  })
})

describe("planRestore", () => {
  it("skips names already taken, ignoring case, including earlier ones in the file", () => {
    expect(planRestore(backup, [" ops "])).toEqual([
      { name: "Ops", restore: false },
      { name: "Research", restore: true },
      { name: "research", restore: false },
    ])
  })
})

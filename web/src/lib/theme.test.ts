import { afterEach, describe, expect, it, vi } from "vitest"

import { readThemePreference, resolveTheme } from "./theme"

async function freshStore() {
  vi.resetModules()
  return (await import("./theme")).useThemeStore
}

describe("theme", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("defaults to system", () => {
    expect(readThemePreference()).toBe("system")
  })

  it("resolves system against the OS preference", () => {
    expect(resolveTheme("system", true)).toBe("dark")
    expect(resolveTheme("system", false)).toBe("light")
    expect(resolveTheme("light", true)).toBe("light")
  })

  it("persists the preference under a namespaced key and applies it to <html>", async () => {
    const store = await freshStore()
    store.getState().setPreference("dark")

    expect(window.localStorage.getItem("openbot:theme")).toBe("dark")
    expect(document.documentElement.classList.contains("dark")).toBe(true)
    expect(store.getState().resolved).toBe("dark")

    store.getState().setPreference("light")
    expect(document.documentElement.classList.contains("dark")).toBe(false)
  })

  it("reads the saved preference on startup and ignores garbage", async () => {
    window.localStorage.setItem("openbot:theme", "dark")
    expect((await freshStore()).getState().preference).toBe("dark")

    window.localStorage.setItem("openbot:theme", "purple")
    expect((await freshStore()).getState().preference).toBe("system")
  })

  it("keeps working when localStorage throws", async () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError")
    })
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError")
    })
    const store = await freshStore()
    expect(store.getState().preference).toBe("system")
    expect(() => store.getState().setPreference("dark")).not.toThrow()
    expect(store.getState().preference).toBe("dark")
  })
})

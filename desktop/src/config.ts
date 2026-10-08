import fs from "node:fs"
import path from "node:path"

import { app } from "electron"

/** What the app remembers between launches. */
export type Config = {
  server?: string
  bounds?: { x: number; y: number; width: number; height: number }
  maximized?: boolean
  /** Keep running (tray, notifications) when the window is closed. */
  closeToTray?: boolean
}

const file = () => path.join(app.getPath("userData"), "config.json")

const get = (from: object, key: string): unknown => Reflect.get(from, key)

export function loadConfig(): Config {
  let raw: unknown
  try {
    raw = JSON.parse(fs.readFileSync(file(), "utf8"))
  } catch {
    return {}
  }
  if (typeof raw !== "object" || raw === null) return {}
  const c: Config = {}
  const server = get(raw, "server")
  if (typeof server === "string") c.server = server
  const maximized = get(raw, "maximized")
  if (typeof maximized === "boolean") c.maximized = maximized
  const closeToTray = get(raw, "closeToTray")
  if (typeof closeToTray === "boolean") c.closeToTray = closeToTray
  const bounds = get(raw, "bounds")
  if (typeof bounds === "object" && bounds !== null) {
    const [x, y, width, height] = ["x", "y", "width", "height"].map((k) => get(bounds, k))
    if (
      typeof x === "number" &&
      typeof y === "number" &&
      typeof width === "number" &&
      typeof height === "number"
    ) {
      c.bounds = { x, y, width, height }
    }
  }
  return c
}

export function saveConfig(config: Config): void {
  try {
    fs.mkdirSync(path.dirname(file()), { recursive: true })
    fs.writeFileSync(file(), JSON.stringify(config, null, 2))
  } catch (err) {
    console.error("save config", err)
  }
}

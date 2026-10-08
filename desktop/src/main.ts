import fs from "node:fs"
import path from "node:path"

// openbot desktop: a window onto the user's own openbot server, plus what a browser tab can't
// do well (native notifications, a tray icon, an unread badge, starting with the system).
import {
  app,
  BrowserWindow,
  ipcMain,
  Menu,
  nativeImage,
  Notification,
  session,
  shell,
  Tray,
  type IpcMainEvent,
  type IpcMainInvokeEvent,
  type MenuItemConstructorOptions,
} from "electron"

import { loadConfig, saveConfig, type Config } from "./config"
import { checkServer, compareVersions, normalizeServer, sameOrigin } from "./server"

const RELEASES = "https://github.com/etchebarne/openbot/releases"
const PARTITION = "persist:openbot"
const assets = path.join(__dirname, "..", "assets")

let config: Config = {}
let win: BrowserWindow | null = null
let tray: Tray | null = null
let quitting = false
let unread = 0
let update: { version: string; url: string } | null = null
let connectError: string | undefined
// --smoke=<file.png>: load, save a screenshot of the window, quit (a check for builds).
const smoke = app.commandLine.getSwitchValue("smoke")
const notifications = new Set<Notification>() // keep a reference, or clicks get lost

if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  config = loadConfig()
  if (config.server) {
    // Servers are often reached over plain http on a private network (Tailscale). Treat the
    // user's own server as a secure context so clipboard, crypto and the like work.
    app.commandLine.appendSwitch("unsafely-treat-insecure-origin-as-secure", config.server)
  }
  app.on("second-instance", show)
  app.on("before-quit", () => {
    quitting = true
  })
  app.on("activate", show) // macOS: clicking the dock icon
  app.on("window-all-closed", () => {
    if (process.platform !== "darwin") app.quit()
  })
  void app.whenReady().then(start)
}

function start() {
  app.setAppUserModelId("net.etchebarne.openbot") // Windows notifications
  secureSession()
  createTray()
  buildMenu()
  createWindow()
  void checkForUpdate()
  setInterval(() => void checkForUpdate(), 6 * 60 * 60 * 1000)
}

const appIcon = () => nativeImage.createFromPath(path.join(assets, "icon.png"))

function createWindow() {
  const b = config.bounds
  win = new BrowserWindow({
    width: b?.width ?? 1200,
    height: b?.height ?? 800,
    x: b?.x,
    y: b?.y,
    minWidth: 360,
    minHeight: 480,
    title: "openbot",
    icon: appIcon(),
    backgroundColor: "#0a0a0a",
    autoHideMenuBar: true,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      partition: PARTITION,
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      spellcheck: true,
    },
  })
  if (config.maximized) win.maximize()
  win.once("ready-to-show", () => {
    if (!smoke) win?.show()
  })
  if (smoke) runSmoke(win, smoke)

  let saveTimer: NodeJS.Timeout | undefined
  const remember = () => {
    clearTimeout(saveTimer)
    saveTimer = setTimeout(() => {
      if (!win || win.isMinimized()) return
      config.maximized = win.isMaximized()
      if (!config.maximized) config.bounds = win.getBounds()
      saveConfig(config)
    }, 500)
  }
  win.on("resize", remember)
  win.on("move", remember)

  win.on("close", (event) => {
    // Keep running in the tray (for notifications) unless the user is quitting.
    if (!quitting && closeToTray() && tray) {
      event.preventDefault()
      win?.hide()
    }
  })
  win.on("closed", () => {
    win = null
  })

  const contents = win.webContents
  // Links to other sites open in the browser; the window only ever shows the user's server
  // (or the local connect page).
  contents.setWindowOpenHandler(({ url }) => {
    if (config.server && sameOrigin(url, config.server)) {
      return {
        action: "allow",
        overrideBrowserWindowOptions: { autoHideMenuBar: true, icon: appIcon() },
      }
    }
    if (/^https?:/i.test(url) || url.startsWith("mailto:")) void shell.openExternal(url)
    return { action: "deny" }
  })
  contents.on("will-navigate", (event, url) => {
    if (url.startsWith("file:") || (config.server && sameOrigin(url, config.server))) return
    event.preventDefault()
    if (/^https?:/i.test(url)) void shell.openExternal(url)
  })
  contents.on("did-fail-load", (_event, code, description, url, isMainFrame) => {
    // -3 is an aborted load (e.g. a navigation replaced it), not a failure.
    if (!isMainFrame || code === -3 || url.startsWith("file:")) return
    showConnect(`Couldn't reach ${config.server}: ${description}.`)
  })
  contents.on("page-title-updated", (event) => {
    event.preventDefault()
    updateTitle()
  })

  if (config.server) void win.loadURL(config.server)
  else showConnect()
}

function closeToTray() {
  return config.closeToTray ?? true
}

function show() {
  if (!win) createWindow()
  if (win?.isMinimized()) win.restore()
  win?.show()
  win?.focus()
}

function showConnect(error?: string) {
  connectError = error
  void win?.loadFile(path.join(__dirname, "connect.html"))
  show()
}

/** Only the user's server may ask for permissions, and only for what the app uses. */
function secureSession() {
  const ses = session.fromPartition(PARTITION)
  const allowed = new Set([
    "clipboard-sanitized-write",
    "clipboard-read",
    "notifications",
    "fullscreen",
  ])
  ses.setPermissionRequestHandler((_wc, permission, callback, details) => {
    callback(
      !!config.server &&
        sameOrigin(details.requestingUrl, config.server) &&
        allowed.has(permission),
    )
  })
  ses.setPermissionCheckHandler((_wc, permission, origin) => {
    return !!config.server && sameOrigin(origin, config.server) && allowed.has(permission)
  })
}

function runSmoke(window: BrowserWindow, file: string) {
  const capture = async () => {
    const image = await window.webContents.capturePage()
    fs.writeFileSync(file, image.toPNG())
    console.log("smoke", window.webContents.getURL())
    console.log(await window.webContents.executeJavaScript("document.body.innerText"))
    console.log(
      "bridge",
      await window.webContents.executeJavaScript("typeof window.openbotDesktop"),
    )
    quitting = true
    app.exit(0)
  }
  window.webContents.once("did-finish-load", () => {
    setTimeout(() => void capture(), 3000)
  })
}

// ---- tray, badge, menu ----

function createTray() {
  const file = process.platform === "darwin" ? "trayTemplate.png" : "tray.png"
  const image = nativeImage.createFromPath(path.join(assets, file))
  try {
    tray = new Tray(image)
  } catch {
    tray = null // no tray on this desktop (e.g. GNOME without an extension)
    return
  }
  tray.on("click", () => (win?.isVisible() && win.isFocused() ? win.hide() : show()))
  updateTray()
}

function updateTray() {
  if (!tray) return
  tray.setToolTip(unread > 0 ? `openbot · ${unread} unread` : "openbot")
  const items: MenuItemConstructorOptions[] = [
    { label: "Open openbot", click: show },
    { type: "separator" },
  ]
  if (update) {
    const target = update
    items.push({
      label: `Download openbot ${target.version}`,
      click: () => void shell.openExternal(target.url),
    })
    items.push({ type: "separator" })
  }
  items.push(
    { label: "Change server…", click: () => showConnect() },
    {
      label: "Keep running when closed",
      type: "checkbox",
      checked: closeToTray(),
      click: (item) => {
        config.closeToTray = item.checked
        saveConfig(config)
      },
    },
    { type: "separator" },
    { label: "Quit openbot", role: "quit" },
  )
  tray.setContextMenu(Menu.buildFromTemplate(items))
}

function updateTitle() {
  win?.setTitle(unread > 0 ? `openbot (${unread})` : "openbot")
}

function buildMenu() {
  const template: MenuItemConstructorOptions[] = [
    ...(process.platform === "darwin" ? [{ role: "appMenu" as const }] : []),
    {
      label: "File",
      submenu: [
        { label: "Change server…", click: () => showConnect() },
        { type: "separator" },
        process.platform === "darwin" ? { role: "close" } : { role: "quit" },
      ],
    },
    { role: "editMenu" },
    {
      label: "View",
      submenu: [
        { role: "reload" },
        { role: "forceReload" },
        { role: "toggleDevTools" },
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "togglefullscreen" },
      ],
    },
    { role: "windowMenu" },
    {
      role: "help",
      submenu: [
        {
          label: "openbot on GitHub",
          click: () => void shell.openExternal("https://github.com/etchebarne/openbot"),
        },
        { label: "Check for updates", click: () => void checkForUpdate(true) },
      ],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

// ---- bridge ----

/** Calls from the web app count only when they come from the user's server. */
function fromServer(event: IpcMainEvent | IpcMainInvokeEvent) {
  const url = event.senderFrame?.url ?? ""
  return !!config.server && sameOrigin(url, config.server)
}

function fromConnectPage(event: IpcMainEvent | IpcMainInvokeEvent) {
  return (event.senderFrame?.url ?? "").startsWith("file:")
}

/** A string from the web app, cut to max characters ("" if it isn't one). */
function text(value: unknown, max: number): string {
  return typeof value === "string" ? value.slice(0, max) : ""
}

ipcMain.on(
  "openbot:notify",
  (event, n: { title?: unknown; body?: unknown; chatId?: unknown; tag?: unknown }) => {
    if (!fromServer(event) || !Notification.isSupported()) return
    const chatId = text(n.chatId, 64)
    const notification = new Notification({
      title: text(n.title, 200) || "openbot",
      body: text(n.body, 1000),
      icon: appIcon(),
    })
    notifications.add(notification)
    notification.on("click", () => {
      show()
      if (chatId) win?.webContents.send("openbot:navigate", `/chats/${encodeURIComponent(chatId)}`)
    })
    notification.on("close", () => notifications.delete(notification))
    notification.show()
  },
)

ipcMain.on("openbot:unread", (event, count: unknown) => {
  if (!fromServer(event) || typeof count !== "number" || !Number.isFinite(count)) return
  unread = Math.max(0, Math.floor(count))
  app.setBadgeCount(unread) // macOS dock, Linux launchers that support it
  updateTitle()
  updateTray()
})

ipcMain.on("openbot:change-server", (event) => {
  if (fromServer(event)) showConnect()
})

ipcMain.handle("openbot:connect-info", (event) => {
  if (!fromConnectPage(event)) return { version: app.getVersion() }
  return { server: config.server, error: connectError, version: app.getVersion() }
})

ipcMain.handle("openbot:connect", async (event, address: unknown) => {
  if (!fromConnectPage(event) || typeof address !== "string")
    return { ok: false, error: "Not allowed." }
  try {
    const origin = normalizeServer(address)
    await checkServer(origin)
    const changed = origin !== config.server
    config.server = origin
    saveConfig(config)
    if (changed) {
      // The secure-context exception is set at startup, so a new server means a restart.
      app.relaunch()
      quitting = true
      app.exit(0)
    } else {
      void win?.loadURL(origin)
    }
    return { ok: true }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
})

// ---- updates ----

async function checkForUpdate(manual = false) {
  if (!app.isPackaged && !manual) return
  try {
    const res = await fetch("https://api.github.com/repos/etchebarne/openbot/releases/latest", {
      headers: { Accept: "application/vnd.github+json" },
      signal: AbortSignal.timeout(15000),
    })
    const latest: unknown = await res.json()
    const field = (key: string) =>
      typeof latest === "object" && latest !== null ? text(Reflect.get(latest, key), 300) : ""
    const tag = field("tag_name")
    const page = field("html_url")
    if (tag && compareVersions(tag, app.getVersion()) > 0) {
      const isNew = update?.version !== tag
      update = { version: tag, url: page.startsWith("https://github.com/") ? page : RELEASES }
      updateTray()
      if (isNew && Notification.isSupported()) {
        const n = new Notification({
          title: `openbot ${tag} is out`,
          body: "Click to download it.",
          icon: appIcon(),
        })
        notifications.add(n)
        n.on("click", () => void shell.openExternal(update?.url ?? RELEASES))
        n.show()
      }
    } else if (manual && Notification.isSupported()) {
      new Notification({
        title: "openbot is up to date",
        body: `You have ${app.getVersion()}.`,
        icon: appIcon(),
      }).show()
    }
  } catch {
    // Offline or rate limited: try again later.
  }
}

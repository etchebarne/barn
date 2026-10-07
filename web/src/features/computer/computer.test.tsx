import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { api, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"
import { makeAgent } from "@/test/fixtures"

import { ComputerPage } from "./computer-page"
import type { FileEntry } from "./paths"

vi.mock("sonner", () => ({
  toast: { success: vi.fn<(t: string) => void>(), error: vi.fn<(t: string) => void>() },
}))

const entry = (name: string, kind: FileEntry["kind"] = "file", size = 12): FileEntry => ({
  name,
  kind,
  size,
  modifiedAt: "2026-10-07T10:00:00Z",
})

const FOLDERS: Record<string, FileEntry[]> = {
  "/home/agent": [entry("notes", "dir", 4096), entry(".bashrc"), entry("todo.md", "file", 11)],
  "/home/agent/notes": [entry("ideas.txt")],
}

type Call = { method: string; path: string; init: unknown }

function paramsOf(init: unknown): { query?: { path?: string } } {
  if (init && typeof init === "object" && "params" in init) {
    const { params } = init
    if (params && typeof params === "object") return params
  }
  return {}
}

/** Answers like the server: sandbox status, folder listings, and 204 for writes. */
function mockApi(sandbox: Schemas["Sandbox"] = { status: "running", sharedWith: [] }) {
  const calls: Call[] = []
  vi.spyOn(api, "GET").mockImplementation((...args: unknown[]) => {
    const [path, init] = args
    calls.push({ method: "GET", path: String(path), init })
    if (String(path).endsWith("/sandbox")) {
      return Promise.resolve({ data: sandbox, error: undefined, response: new Response(null) })
    }
    const folder = paramsOf(init).query?.path ?? ""
    const entries = FOLDERS[folder]
    if (!entries) {
      return Promise.resolve({
        data: undefined,
        error: { message: "not found" },
        response: new Response(null, { status: 404 }),
      })
    }
    return Promise.resolve({
      data: { path: folder, entries, truncated: false },
      error: undefined,
      response: new Response(null),
    })
  })
  const write =
    (method: string) =>
    (...args: unknown[]) => {
      const [path, init] = args
      calls.push({ method, path: String(path), init })
      return Promise.resolve({
        data: undefined,
        error: undefined,
        response: new Response(null, { status: 204 }),
      })
    }
  vi.spyOn(api, "POST").mockImplementation(write("POST"))
  vi.spyOn(api, "DELETE").mockImplementation(write("DELETE"))
  return calls
}

/** Records XHR PUTs (uploads and saves). */
const puts: { url: string; body: unknown; headers: Record<string, string> }[] = []
class FakeXhr {
  status = 0
  responseText = ""
  upload = { addEventListener: () => {} }
  private url = ""
  private headers: Record<string, string> = {}
  private listeners: Record<string, () => void> = {}
  open(_method: string, url: string) {
    this.url = url
  }
  setRequestHeader(name: string, value: string) {
    this.headers[name] = value
  }
  addEventListener(type: string, listener: () => void) {
    this.listeners[type] = listener
  }
  send(body: unknown) {
    puts.push({ url: this.url, body, headers: this.headers })
    this.status = 204
    queueMicrotask(() => this.listeners.load?.())
  }
  abort() {}
}

async function renderComputer(sandbox?: Schemas["Sandbox"]) {
  const calls = mockApi(sandbox)
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  client.setQueryData(queryKeys.agents, [makeAgent({ id: "research", name: "Research" })])
  const rootRoute = createRootRoute({
    component: () => (
      <TooltipProvider>
        <ComputerPage agentId="research" />
      </TooltipProvider>
    ),
  })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  return calls
}

const folderGets = (calls: Call[]) =>
  calls
    .filter((c) => c.method === "GET" && c.path.endsWith("/files"))
    .map((c) => paramsOf(c.init).query?.path)

beforeEach(() => {
  puts.length = 0
  vi.stubGlobal("XMLHttpRequest", FakeXhr)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe("Computer: files", () => {
  it("lists /home/agent without dotfiles, and opens folders with the keyboard", async () => {
    const calls = await renderComputer()
    const list = await screen.findByRole("listbox", { name: "Files in /home/agent" })
    expect(await within(list).findByRole("option", { name: /todo\.md/ })).toBeVisible()
    expect(within(list).queryByText(".bashrc")).not.toBeInTheDocument()
    expect(screen.getByText("Running")).toBeVisible()

    list.focus()
    await userEvent.keyboard("{ArrowDown}{Enter}")
    expect(await screen.findByRole("listbox", { name: "Files in /home/agent/notes" })).toBeVisible()
    expect(await screen.findByText("ideas.txt")).toBeVisible()
    expect(folderGets(calls)).toEqual(["/home/agent", "/home/agent/notes"])

    // Backspace goes up, and the breadcrumb navigates too.
    screen.getByRole("listbox").focus()
    await userEvent.keyboard("{Backspace}")
    expect(await screen.findByRole("listbox", { name: "Files in /home/agent" })).toBeVisible()
    await userEvent.click(
      within(screen.getByRole("navigation", { name: "Folder path" })).getByRole("button", {
        name: "home",
      }),
    )
    await waitFor(() => expect(folderGets(calls).at(-1)).toBe("/home"))
  })

  it("shows dotfiles when asked, and remembers it", async () => {
    await renderComputer()
    await screen.findByText("todo.md")
    await userEvent.click(screen.getByRole("button", { name: "Show hidden files" }))
    expect(screen.getByText(".bashrc")).toBeVisible()
    expect(window.localStorage.getItem("openbot:computer:show-hidden")).toBe("1")
  })

  it("uploads files with a PUT of their bytes to the folder", async () => {
    await renderComputer()
    await screen.findByText("todo.md")
    const file = new File(["hello"], "hello.txt", { type: "text/plain" })
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')
    if (!input) throw new Error("no file input")
    fireEvent.change(input, { target: { files: [file] } })
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]?.url).toBe("/api/agents/research/sandbox/files?path=%2Fhome%2Fagent%2Fhello.txt")
    expect(puts[0]?.body).toBe(file)
    expect(puts[0]?.headers["X-Openbot-CSRF"]).toBe("1")
  })

  it("renames with F2 by moving the file", async () => {
    const calls = await renderComputer()
    await screen.findByText("todo.md")
    const list = screen.getByRole("listbox")
    await userEvent.click(within(list).getByText("todo.md"))
    list.focus()
    await userEvent.keyboard("{F2}")
    const field = screen.getByRole("textbox", { name: "Rename todo.md" })
    await userEvent.clear(field)
    await userEvent.type(field, "done.md{Enter}")
    expect(calls.filter((c) => c.method === "POST")).toEqual([
      {
        method: "POST",
        path: "/agents/{agentId}/sandbox/move",
        init: {
          params: { path: { agentId: "research" } },
          body: { from: "/home/agent/todo.md", to: "/home/agent/done.md" },
        },
      },
    ])
  })

  it("deletes after confirming, and says a folder goes with everything in it", async () => {
    const calls = await renderComputer()
    await screen.findByText("notes")
    fireEvent.contextMenu(within(screen.getByRole("listbox")).getByText("notes"))
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete" }))
    const dialog = await screen.findByRole("alertdialog")
    expect(dialog).toHaveTextContent("Delete notes?")
    expect(dialog).toHaveTextContent("and everything in it")
    expect(calls.some((c) => c.method === "DELETE")).toBe(false)
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }))
    await waitFor(() =>
      expect(
        calls.filter((c) => c.method === "DELETE").map((c) => paramsOf(c.init).query?.path),
      ).toEqual(["/home/agent/notes"]),
    )
  })

  it("edits a text file and saves it with Ctrl+S", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(() => Promise.resolve(new Response("# Todo\n- one\n"))),
    )
    await renderComputer()
    await userEvent.dblClick(await screen.findByText("todo.md"))
    const editor = await screen.findByRole("textbox", { name: "Contents of todo.md" })
    expect(editor).toHaveValue("# Todo\n- one\n")
    await userEvent.type(editor, "- two")
    expect(screen.getByText("Unsaved changes")).toBeVisible()
    await userEvent.keyboard("{Control>}s{/Control}")
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]?.url).toContain("path=%2Fhome%2Fagent%2Ftodo.md")
    expect(puts[0]?.body).toBe("# Todo\n- one\n- two")
    expect(await screen.findByText("Saved")).toBeVisible()
  })
})

describe("Computer: unavailable", () => {
  it("explains that Docker isn't available, with no tabs", async () => {
    await renderComputer({ status: "unavailable", sharedWith: [] })
    expect(await screen.findByText("No computer on this server")).toBeVisible()
    expect(screen.getByText(/Docker isn't available on this server/)).toBeVisible()
    expect(screen.queryByRole("tab")).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /Restart/ })).not.toBeInTheDocument()
  })
})

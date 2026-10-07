import { QueryClient, QueryClientProvider, useMutation } from "@tanstack/react-query"
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { ApiError } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

import { AddConnection, apiKeyNote } from "./add-connection"
import { CatalogList } from "./catalog"
import { SignInStatus } from "./connector-detail"
import type { Connector, ConnectorType } from "./logic"
import { SignInButton } from "./sign-in-button"
import { useSignInReturn } from "./sign-in-return"
import {
  launchSignIn,
  readSignInReturn,
  signInReturnMessage,
  type CatalogApp,
  type SignInDeps,
} from "./signin"

const toast = vi.hoisted(() => ({
  success: vi.fn<(text: string) => void>(),
  error: vi.fn<(text: string) => void>(),
}))
vi.mock("sonner", () => ({ toast }))

// Creating an MCP server with only a URL is answered with "sign_in_required".
vi.mock("./api", async (importOriginal) => {
  const original = await importOriginal<typeof import("./api")>()
  return {
    ...original,
    useCreateConnector: () =>
      useMutation({
        mutationFn: async (): Promise<Connector> => {
          throw new ApiError(400, "This server needs a sign-in.", "sign_in_required")
        },
      }),
  }
})

const catalog: CatalogApp[] = [
  {
    id: "linear",
    name: "Linear",
    description: "Issues and projects",
    url: "https://mcp.linear.app/mcp",
  },
  {
    id: "notion",
    name: "Notion",
    description: "Docs and wikis",
    url: "https://mcp.notion.com/mcp",
  },
]

const mcpType: ConnectorType = {
  type: "mcp",
  name: "MCP server",
  description: "Any MCP server",
  credentialFields: [
    { key: "apiKey", label: "API key", secret: true, optional: true, events: false },
  ],
  configFields: [
    { key: "url", label: "Server URL", secret: false, optional: false, events: false },
  ],
  signals: [],
  webhooks: false,
  setup: { steps: [], eventSteps: [] },
}
const linearKey: ConnectorType = {
  ...mcpType,
  type: "linear",
  name: "Linear",
  description: "With an API key",
}

const signedIn: Connector = {
  id: "k1",
  type: "mcp",
  name: "Linear",
  config: { url: "https://mcp.linear.app/mcp" },
  credentialsSet: [],
  agentIds: [],
  webhookUrl: null,
  webhookSecret: null,
  signIn: "ok",
  createdAt: "2026-10-01T00:00:00Z",
}

function fakeDeps(pasteBack: boolean): SignInDeps & {
  navigate: { assign: ReturnType<typeof vi.fn>; open: ReturnType<typeof vi.fn> }
} {
  return {
    start: vi.fn<SignInDeps["start"]>().mockResolvedValue({
      authorizeUrl: "https://linear.app/oauth/authorize?x=1",
      pasteBack,
    }),
    complete: vi
      .fn<SignInDeps["complete"]>()
      .mockResolvedValue({ connector: signedIn, chatId: null }),
    navigate: { assign: vi.fn<(url: string) => void>(), open: vi.fn<(url: string) => void>() },
  }
}

async function renderApp(ui: ReactNode, seed?: (client: QueryClient) => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(queryKeys.agents, [])
  seed?.(client)
  const rootRoute = createRootRoute({ component: () => <TooltipProvider>{ui}</TooltipProvider> })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={client}>
      {/* oxlint-disable-next-line typescript/no-unsafe-type-assertion -- a bare test router */}
      <RouterProvider router={router as never} />
    </QueryClientProvider>,
  )
  await waitFor(() => expect(document.body.textContent).not.toBe(""))
  return client
}

describe("launchSignIn", () => {
  it("goes to the app in this tab normally, or a new tab for paste-back", () => {
    const nav = { assign: vi.fn<(u: string) => void>(), open: vi.fn<(u: string) => void>() }
    expect(launchSignIn({ authorizeUrl: "https://a", pasteBack: false }, nav)).toBe("redirect")
    expect(nav.assign).toHaveBeenCalledWith("https://a")
    expect(launchSignIn({ authorizeUrl: "https://b", pasteBack: true }, nav)).toBe("paste")
    expect(nav.open).toHaveBeenCalledWith("https://b")
  })
})

describe("catalog", () => {
  it("lists sign-in apps with their descriptions", async () => {
    await renderApp(<CatalogList onPick={vi.fn<(app: CatalogApp) => void>()} />, (c) =>
      c.setQueryData(queryKeys.connectorCatalog, catalog),
    )
    const list = screen.getByRole("list", { name: "Sign in with" })
    expect(list).toHaveTextContent("Linear")
    expect(list).toHaveTextContent("Issues and projects")
    expect(list).toHaveTextContent("Notion")
  })

  it("explains the API-key variant of an app that also has sign-in", () => {
    expect(apiKeyNote(linearKey, catalog)).toBe("API key · needed for webhooks and events")
    expect(apiKeyNote(mcpType, catalog)).toBeNull()
  })

  it("signs in from the catalog with the name and agents", async () => {
    await renderApp(
      <AddConnection onDone={vi.fn<() => void>()} onOpen={vi.fn<(id: string) => void>()} />,
      (c) => {
        c.setQueryData(queryKeys.connectorCatalog, catalog)
        c.setQueryData(queryKeys.connectorTypes, [mcpType, linearKey])
      },
    )
    expect(screen.getByRole("heading", { name: "Other apps" })).toBeVisible()
    expect(screen.getByText("API key · needed for webhooks and events")).toBeVisible()

    await userEvent.click(screen.getByRole("button", { name: /Notion/ }))
    expect(screen.getByLabelText("Name")).toHaveValue("Notion")
    expect(screen.getByRole("button", { name: "Sign in with Notion" })).toBeVisible()
  })
})

describe("SignInButton", () => {
  it("redirects in this tab normally", async () => {
    const deps = fakeDeps(false)
    await renderApp(
      <SignInButton appName="Linear" request={{ url: "u", name: "Linear" }} deps={deps} />,
    )
    await userEvent.click(screen.getByRole("button", { name: "Sign in with Linear" }))
    expect(deps.start).toHaveBeenCalledWith({ url: "u", name: "Linear" })
    expect(deps.navigate.assign).toHaveBeenCalledWith("https://linear.app/oauth/authorize?x=1")
    expect(deps.navigate.open).not.toHaveBeenCalled()
  })

  it("opens a new tab and finishes with the pasted address", async () => {
    const deps = fakeDeps(true)
    const onCompleted = vi.fn<(r: unknown) => void>()
    await renderApp(
      <SignInButton
        appName="Linear"
        request={{ url: "u" }}
        deps={deps}
        onCompleted={onCompleted}
      />,
    )
    await userEvent.click(screen.getByRole("button", { name: "Sign in with Linear" }))
    expect(deps.navigate.open).toHaveBeenCalled()
    expect(screen.getByText("Click Allow on Linear's page.")).toBeVisible()
    expect(screen.getByText(/open barn over HTTPS/)).toBeVisible()

    await userEvent.type(screen.getByRole("textbox"), "http://localhost:1/oauth/callback?code=abc")
    await userEvent.click(screen.getByRole("button", { name: "Finish" }))
    expect(deps.complete).toHaveBeenCalledWith("http://localhost:1/oauth/callback?code=abc")
    expect(onCompleted).toHaveBeenCalledWith({ connector: signedIn, chatId: null })
  })

  it("shows a rejected paste inline", async () => {
    const deps = fakeDeps(true)
    vi.mocked(deps.complete).mockRejectedValue(
      new ApiError(400, "That address has no sign-in code."),
    )
    await renderApp(<SignInButton appName="Linear" request={{ url: "u" }} deps={deps} />)
    await userEvent.click(screen.getByRole("button", { name: "Sign in with Linear" }))
    await userEvent.type(screen.getByRole("textbox"), "http://nope")
    await userEvent.click(screen.getByRole("button", { name: "Finish" }))
    expect(await screen.findByText("That address has no sign-in code.")).toBeVisible()
    expect(screen.getByRole("button", { name: "Finish" })).toBeEnabled()
  })
})

describe("sign_in_required", () => {
  it("switches the MCP server form to Sign in, using the form's URL", async () => {
    await renderApp(
      <AddConnection onDone={vi.fn<() => void>()} onOpen={vi.fn<(id: string) => void>()} />,
      (c) => {
        c.setQueryData(queryKeys.connectorCatalog, [])
        c.setQueryData(queryKeys.connectorTypes, [mcpType])
      },
    )
    await userEvent.click(screen.getByRole("button", { name: /MCP server/ }))
    await userEvent.type(screen.getByLabelText(/Server URL/), "https://mcp.example.com/mcp")
    await userEvent.click(screen.getByRole("button", { name: "Connect" }))

    expect(await screen.findByText("This server uses sign-in")).toBeVisible()
    expect(screen.queryByText("This server needs a sign-in.")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Sign in" })).toBeVisible()
  })
})

describe("signed-in connections", () => {
  it("show Signed in with Reconnect", async () => {
    await renderApp(<SignInStatus connector={signedIn} type={mcpType} />)
    expect(screen.getByText(/Signed in/)).toBeVisible()
    expect(screen.getByText("https://mcp.linear.app/mcp")).toBeVisible()
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeVisible()
  })

  it("warn when the sign-in expired, and reconnect by id", async () => {
    const deps = fakeDeps(false)
    await renderApp(
      <SignInStatus connector={{ ...signedIn, signIn: "expired" }} type={mcpType} deps={deps} />,
    )
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Linear's sign-in expired. Agents can't use it until you reconnect.",
    )
    await userEvent.click(screen.getByRole("button", { name: "Reconnect" }))
    expect(deps.start).toHaveBeenCalledWith({ connectorId: "k1" })
    expect(deps.navigate.assign).toHaveBeenCalled()
  })
})

function Probe({
  params,
  clear,
}: {
  params: Parameters<typeof useSignInReturn>[0]
  clear: () => void
}) {
  useSignInReturn(params, clear)
  return <p>probe</p>
}

describe("returning from sign-in", () => {
  it("reads only the sign-in params", () => {
    expect(readSignInReturn({ connected: "1", signin_error: "", x: 1 })).toEqual({ connected: "1" })
    expect(readSignInReturn({ signin_error: "Access denied" })).toEqual({
      signin_error: "Access denied",
    })
  })

  it("names the connection in the toast", () => {
    expect(signInReturnMessage({ connected: "1", connector: "k1" }, [signedIn])).toEqual({
      kind: "success",
      text: "Connected Linear",
    })
    expect(signInReturnMessage({ connected: "k1" }, [signedIn])?.text).toBe("Connected Linear")
    expect(signInReturnMessage({ signin_error: "Access denied" }, [])).toEqual({
      kind: "error",
      text: "Access denied",
    })
    expect(signInReturnMessage({}, [])).toBeNull()
  })

  it("toasts once and clears the params", async () => {
    const clear = vi.fn<() => void>()
    await renderApp(<Probe params={{ connected: "1", connector: "k1" }} clear={clear} />, (c) =>
      c.setQueryData(queryKeys.connectors, [signedIn]),
    )
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Connected Linear"))
    expect(clear).toHaveBeenCalledTimes(1)
  })

  it("toasts sign-in errors", async () => {
    const clear = vi.fn<() => void>()
    await renderApp(<Probe params={{ signin_error: "Linear said no" }} clear={clear} />)
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Linear said no"))
    expect(clear).toHaveBeenCalled()
  })
})

import { useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { LogInIcon } from "lucide-react"
import { useState, type FormEvent } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { queryKeys } from "@/lib/query-keys"

import {
  defaultSignInDeps,
  launchSignIn,
  type SignInDeps,
  type SignInResult,
  type StartSignInRequest,
} from "./signin"

/** After a pasted-back sign-in: refresh, toast, and go where the redirect would have gone. */
export function useSignInCompleted() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  return (result: SignInResult) => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.connectors })
    toast.success(`Connected ${result.connector.name}`)
    if (result.chatId) {
      void queryClient.invalidateQueries({ queryKey: queryKeys.messages(result.chatId) })
      void navigate({ to: "/chats/$chatId", params: { chatId: result.chatId } })
    } else {
      void navigate({ to: "/settings", search: { connector: result.connector.id } })
    }
  }
}

/** The paste-back step: the app couldn't send the user back to barn, so they bring the address. */
function PasteBack({
  appName,
  complete,
  onCompleted,
  onCancel,
}: {
  appName: string
  complete: SignInDeps["complete"]
  onCompleted: (result: SignInResult) => void
  onCancel: () => void
}) {
  const [url, setUrl] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function finish(event: FormEvent) {
    event.preventDefault()
    if (!url.trim()) return
    setPending(true)
    setError(null)
    try {
      const result = await complete(url.trim())
      onCompleted(result)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Couldn't finish signing in.")
    } finally {
      setPending(false)
    }
  }

  return (
    <form
      onSubmit={(e) => void finish(e)}
      aria-label={`Finish signing in to ${appName}`}
      className="flex flex-col gap-3 rounded-[calc(var(--radius-md)+0.75rem)] border bg-muted/40 p-3 text-sm"
    >
      <ol className="flex list-decimal flex-col gap-1 pl-5">
        <li>Click Allow on {appName}'s page.</li>
        <li>You'll land on a page that can't be reached. That's expected.</li>
        <li>Copy that page's address from the address bar and paste it here.</li>
      </ol>
      <Field data-invalid={!!error || undefined}>
        <FieldLabel htmlFor={`paste-back-${appName}`} className="sr-only">
          Address of the page you landed on
        </FieldLabel>
        <Input
          id={`paste-back-${appName}`}
          autoComplete="off"
          autoFocus
          placeholder="http://localhost/oauth/callback?code=…"
          value={url}
          disabled={pending}
          aria-invalid={!!error || undefined}
          onChange={(e) => {
            setUrl(e.target.value)
            setError(null)
          }}
        />
        {error && <FieldError>{error}</FieldError>}
      </Field>
      <div className="flex items-center justify-end gap-2">
        <Button type="button" size="sm" variant="ghost" disabled={pending} onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={pending || !url.trim()}>
          {pending && <Spinner />}
          Finish
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        To skip this step, open barn over HTTPS (for example with Tailscale Serve).
      </p>
    </form>
  )
}

/**
 * "Sign in with <app>": starts the app's sign-in. Usually this leaves barn and comes back
 * connected; when the app can't redirect back, it opens a new tab and shows the paste-back
 * step right here (inline, so no dialog stacks on a sheet).
 */
export function SignInButton({
  request,
  appName,
  label = `Sign in with ${appName}`,
  variant = "default",
  size = "sm",
  disabled,
  deps = defaultSignInDeps,
  onCompleted,
}: {
  /** The sign-in request, or a function building it when clicked (e.g. from form values). */
  request: StartSignInRequest | (() => StartSignInRequest)
  appName: string
  label?: string
  variant?: "default" | "outline" | "secondary"
  size?: "sm" | "default"
  disabled?: boolean
  deps?: SignInDeps
  /** Defaults to toasting and navigating like barn's own redirect would. */
  onCompleted?: (result: SignInResult) => void
}) {
  const completed = useSignInCompleted()
  const [starting, setStarting] = useState(false)
  const [pasting, setPasting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function start() {
    setStarting(true)
    setError(null)
    try {
      const body = typeof request === "function" ? request() : request
      const response = await deps.start(body)
      if (launchSignIn(response, deps.navigate) === "paste") setPasting(true)
      // Otherwise the page is navigating away; keep the pending state until it does.
      else return
    } catch (e) {
      setError(e instanceof Error ? e.message : "Couldn't start signing in.")
    }
    setStarting(false)
  }

  return (
    <div className="flex flex-col gap-3">
      {!pasting && (
        <div className="flex flex-col gap-1.5">
          <Button
            type="button"
            variant={variant}
            size={size}
            className="w-fit"
            disabled={disabled || starting}
            onClick={() => void start()}
          >
            {starting ? <Spinner /> : <LogInIcon />}
            {label}
          </Button>
          {error && (
            <p role="alert" className="text-xs text-destructive">
              {error}
            </p>
          )}
        </div>
      )}
      {pasting && (
        <PasteBack
          appName={appName}
          complete={deps.complete}
          onCompleted={(result) => {
            setPasting(false)
            ;(onCompleted ?? completed)(result)
          }}
          onCancel={() => setPasting(false)}
        />
      )}
    </div>
  )
}

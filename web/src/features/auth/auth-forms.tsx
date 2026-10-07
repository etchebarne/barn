import { useState, type FormEvent } from "react"

import { SecretInput } from "@/components/secret-input"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

import { useLogin, useSetupAccount } from "./api"
import { MIN_PASSWORD_LENGTH, validateSetup, type SetupErrors } from "./validation"

function FormError({ error }: { error: Error | null }) {
  if (!error) return null
  return <FieldError>{error.message}</FieldError>
}

export function SetupForm({ onDone }: { onDone: () => void }) {
  const setup = useSetupAccount()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [errors, setErrors] = useState<SetupErrors>({})

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    const next = validateSetup({ username, password, confirm })
    setErrors(next)
    if (Object.keys(next).length > 0) return
    setup.mutate({ username: username.trim(), password }, { onSuccess: onDone })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Create your account</CardTitle>
        <CardDescription>
          openbot is single-user. This account is the only one, so pick a strong password.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} noValidate>
          <FieldGroup>
            <Field data-invalid={!!errors.username || undefined}>
              <FieldLabel htmlFor="setup-username">Username</FieldLabel>
              <Input
                id="setup-username"
                autoComplete="username"
                autoCapitalize="off"
                autoFocus
                value={username}
                aria-invalid={!!errors.username || undefined}
                onChange={(e) => setUsername(e.target.value)}
              />
              <FieldError>{errors.username}</FieldError>
            </Field>
            <Field data-invalid={!!errors.password || undefined}>
              <FieldLabel htmlFor="setup-password">Password</FieldLabel>
              <SecretInput
                id="setup-password"
                autoComplete="new-password"
                value={password}
                aria-invalid={!!errors.password || undefined}
                onChange={(e) => setPassword(e.target.value)}
              />
              {errors.password ? (
                <FieldError>{errors.password}</FieldError>
              ) : (
                <FieldDescription>At least {MIN_PASSWORD_LENGTH} characters.</FieldDescription>
              )}
            </Field>
            <Field data-invalid={!!errors.confirm || undefined}>
              <FieldLabel htmlFor="setup-confirm">Confirm password</FieldLabel>
              <SecretInput
                id="setup-confirm"
                autoComplete="new-password"
                value={confirm}
                aria-invalid={!!errors.confirm || undefined}
                onChange={(e) => setConfirm(e.target.value)}
              />
              <FieldError>{errors.confirm}</FieldError>
            </Field>
            <FormError error={setup.error} />
            <Button type="submit" size="lg" disabled={setup.isPending}>
              {setup.isPending && <Spinner />}
              Create account
            </Button>
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  )
}

export function LoginForm({ onDone }: { onDone: () => void }) {
  const login = useLogin()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")

  function onSubmit(event: FormEvent) {
    event.preventDefault()
    if (!username.trim() || !password) return
    login.mutate({ username: username.trim(), password }, { onSuccess: onDone })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Sign in</CardTitle>
        <CardDescription>Welcome back.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="login-username">Username</FieldLabel>
              <Input
                id="login-username"
                autoComplete="username"
                autoCapitalize="off"
                autoFocus
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="login-password">Password</FieldLabel>
              <SecretInput
                id="login-password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <FormError error={login.error} />
            <Button type="submit" size="lg" disabled={login.isPending}>
              {login.isPending && <Spinner />}
              Sign in
            </Button>
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  )
}

export const MIN_PASSWORD_LENGTH = 12

export type SetupErrors = Partial<Record<"username" | "password" | "confirm", string>>

export function validateSetup(values: {
  username: string
  password: string
  confirm: string
}): SetupErrors {
  const errors: SetupErrors = {}
  if (!values.username.trim()) errors.username = "Choose a username."
  else if (values.username.length > 64) errors.username = "Use at most 64 characters."
  if (values.password.length < MIN_PASSWORD_LENGTH) {
    errors.password = `Use at least ${MIN_PASSWORD_LENGTH} characters.`
  } else if (values.password.length > 256) {
    errors.password = "Use at most 256 characters."
  }
  if (!errors.password && values.confirm !== values.password) {
    errors.confirm = "Passwords don't match."
  }
  return errors
}

import { useSuspenseQuery } from "@tanstack/react-query"

import { FirstRunLayout } from "@/components/first-run-layout"

import { authStatusQueryOptions } from "./api"
import { LoginForm, SetupForm } from "./auth-forms"

/** Account setup on first run, login afterwards. */
export function AuthScreen({ onAuthenticated }: { onAuthenticated: () => void }) {
  const { data } = useSuspenseQuery(authStatusQueryOptions)
  return (
    <FirstRunLayout stepKey={data.setupRequired ? "setup" : "login"}>
      {data.setupRequired ? (
        <SetupForm onDone={onAuthenticated} />
      ) : (
        <LoginForm onDone={onAuthenticated} />
      )}
    </FirstRunLayout>
  )
}

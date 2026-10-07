import { useSuspenseQuery } from "@tanstack/react-query"
import { useState } from "react"

import { FirstRunLayout } from "@/components/first-run-layout"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { OPENCODE_GO_DOCS_URL, ProviderKeyForm } from "@/features/settings"

import { AgentStep } from "./agent-step"
import { onboardingQueryOptions } from "./api"

function StepLabel({ current, total }: { current: number; total: number }) {
  if (total < 2) return null
  return (
    <p className="text-xs font-medium text-muted-foreground tabular-nums">
      Step {current} of {total}
    </p>
  )
}

/**
 * First run, after the account exists: connect OpenCode Go (skipped if a key is already
 * saved), then create the starter agent.
 */
export function OnboardingScreen({ onComplete }: { onComplete: (chatId: string) => void }) {
  const { data } = useSuspenseQuery(onboardingQueryOptions)
  // If a key was saved before this visit, the provider step is skipped: one step in total.
  const [total] = useState(() => (data.providerConfigured ? 1 : 2))
  const step = data.providerConfigured ? "agent" : "provider"

  return (
    <FirstRunLayout stepKey={step}>
      <Card>
        {step === "provider" ? (
          <>
            <CardHeader>
              <StepLabel current={1} total={2} />
              <CardTitle>Connect OpenCode Go</CardTitle>
              <CardDescription>
                Your agents think with models from your{" "}
                <a
                  href={OPENCODE_GO_DOCS_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-4 hover:text-foreground"
                >
                  OpenCode Go
                </a>{" "}
                subscription.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <ProviderKeyForm autoFocus submitLabel="Connect" />
            </CardContent>
          </>
        ) : (
          <>
            <CardHeader>
              <StepLabel current={total} total={total} />
              <CardTitle>Create your first agent</CardTitle>
              <CardDescription>
                It's your first coworker. You'll create every other agent by talking to it.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AgentStep onComplete={onComplete} />
            </CardContent>
          </>
        )}
      </Card>
    </FirstRunLayout>
  )
}

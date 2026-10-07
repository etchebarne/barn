import { EyeIcon, EyeOffIcon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"

import { CopyButton } from "@/components/copy-button"
import { Button } from "@/components/ui/button"

import {
  maskSecret,
  maskWebhookUrl,
  webhookUrlIsSecret,
  type Connector,
  type ConnectorType,
} from "./logic"

/** openbot's generated webhook signing secret: masked until revealed, with Copy. */
export function WebhookSecret({ connector, type }: { connector: Connector; type?: ConnectorType }) {
  const [revealed, setRevealed] = useState(false)
  const secret = connector.webhookSecret
  if (!secret) return null
  const service = type?.name ?? "the service"
  return (
    <section className="flex flex-col gap-2" aria-labelledby={`webhook-secret-${connector.id}`}>
      <h3 id={`webhook-secret-${connector.id}`} className="text-sm font-medium">
        Webhook secret
      </h3>
      <div className="flex items-center gap-1 rounded-lg border bg-muted/40 py-1 pr-1 pl-3">
        <code className="min-w-0 flex-1 truncate font-mono text-xs" data-testid="webhook-secret">
          {revealed ? secret : maskSecret()}
        </code>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={revealed ? "Hide webhook secret" : "Show webhook secret"}
          aria-pressed={revealed}
          onClick={() => setRevealed((v) => !v)}
        >
          {revealed ? <EyeOffIcon /> : <EyeIcon />}
        </Button>
        <CopyButton text={secret} label="Copy webhook secret" />
      </div>
      <p className="text-xs text-muted-foreground">
        Paste this into {service}'s webhook settings so openbot can verify events come from it.
      </p>
    </section>
  )
}

/** Where the service sends events: the URL with Copy, masked until revealed when it's secret. */
export function WebhookInfo({ connector, type }: { connector: Connector; type?: ConnectorType }) {
  const [revealed, setRevealed] = useState(false)
  const url = connector.webhookUrl
  if (!url) return null
  const secret = webhookUrlIsSecret(connector.type)
  const service = type?.name ?? "the service"

  return (
    <section className="flex flex-col gap-2" aria-labelledby={`webhook-${connector.id}`}>
      <h3 id={`webhook-${connector.id}`} className="text-sm font-medium">
        Webhook URL
      </h3>
      <div className="flex items-center gap-1 rounded-lg border bg-muted/40 py-1 pr-1 pl-3">
        <code
          className="min-w-0 flex-1 truncate font-mono text-xs"
          title={secret && !revealed ? undefined : url}
        >
          {secret && !revealed ? maskWebhookUrl(url) : url}
        </code>
        {secret && (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={revealed ? "Hide webhook URL" : "Show webhook URL"}
            aria-pressed={revealed}
            onClick={() => setRevealed((v) => !v)}
          >
            {revealed ? <EyeOffIcon /> : <EyeIcon />}
          </Button>
        )}
        <CopyButton text={url} label="Copy webhook URL" />
      </div>
      <p className="text-xs text-muted-foreground">
        {secret
          ? "Send events to this URL with a POST request."
          : `Paste this into ${service}'s webhook settings.`}
      </p>
      {secret && (
        <p className="flex items-start gap-1.5 text-xs text-warning-foreground">
          <TriangleAlertIcon className="mt-px size-3.5 shrink-0" aria-hidden="true" />
          This URL contains a secret token. Anyone who has it can send events to your agents.
        </p>
      )}
    </section>
  )
}

/** What agents can react to from this kind of connection. */
export function SignalsList({ type }: { type?: ConnectorType }) {
  if (!type || type.signals.length === 0) return null
  return (
    <section className="flex flex-col gap-2" aria-labelledby={`signals-${type.type}`}>
      <h3 id={`signals-${type.type}`} className="text-sm font-medium">
        Events agents can react to
      </h3>
      <ul className="flex flex-col gap-1.5">
        {type.signals.map((signal) => (
          <li key={signal.type} className="flex flex-col text-sm">
            <code className="font-mono text-xs">{signal.type}</code>
            <span className="text-muted-foreground">{signal.description}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

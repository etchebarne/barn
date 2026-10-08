import { useNavigate } from "@tanstack/react-router"
import { cn } from "cn"
import { ChevronLeftIcon, MessageSquareIcon, MonitorIcon, ShieldCheckIcon } from "lucide-react"
import { useRef, useState, type ReactNode, type Ref } from "react"
import { toast } from "sonner"

import { ModelPicker } from "@/components/model-picker"
import { SegmentedControl } from "@/components/segmented-control"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import { AgentConnections } from "@/features/connectors"
import { AgentUsageSection } from "@/features/usage"
import { useIsMobile } from "@/hooks/use-mobile"
import type { Agent } from "@/lib/api-client"
import { useAgentDm } from "@/lib/chat-history"

import { activityLabel } from "./activity"
import { AgentAvatar } from "./agent-avatar"
import { useAgentsById, useMemories, useTasks, useUpdateAgent } from "./api"
import { ComputerSection } from "./computer-section"
import { useAgentDetailsStore } from "./details-store"
import { MemoriesSection } from "./memories-section"
import { SecretsSection } from "./secrets-section"
import {
  DangerZone,
  InstructionsSection,
  PersonalitySection,
  LanguageSection,
  NameSection,
  NotificationsSection,
  TrustSection,
} from "./settings-sections"
import { StandingApprovalsSection } from "./standing-approvals-section"
import { TasksSection } from "./tasks-section"

/** Model setting: changes apply as soon as a model is picked. */
function ModelField({ agent, inputRef }: { agent: Agent; inputRef?: Ref<HTMLInputElement> }) {
  const update = useUpdateAgent(agent.id)
  // While saving (or after a failed save) show the chosen model; otherwise the server's.
  const [choice, setChoice] = useState<string | null>(null)
  const value = choice ?? agent.model

  function onValueChange(model: string | null) {
    if (!model || model === agent.model) {
      setChoice(null)
      update.reset()
      return
    }
    setChoice(model)
    update.mutate(
      { model },
      {
        onSuccess: (updated) => {
          setChoice(null)
          toast.success(`${updated.name} now uses ${updated.model}`)
        },
      },
    )
  }

  return (
    <Field data-invalid={!!update.error || undefined}>
      <FieldLabel htmlFor="agent-details-model" className="gap-2">
        Model
        {update.isPending && <Spinner className="size-3.5 text-muted-foreground" />}
      </FieldLabel>
      <ModelPicker
        id="agent-details-model"
        inputRef={inputRef}
        value={value}
        invalid={!!update.error}
        disabled={update.isPending}
        onValueChange={onValueChange}
      />
      {update.error ? (
        <FieldError>{update.error.message}</FieldError>
      ) : (
        <FieldDescription>Used for every model call this agent makes.</FieldDescription>
      )}
    </Field>
  )
}

type Tab = "profile" | "behavior" | "memory" | "tasks" | "access" | "usage"

/** A titled group of settings on its own surface, so each tab reads as a few clear blocks. */
function Card({
  title,
  description,
  children,
  className,
}: {
  title?: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("flex flex-col gap-4 rounded-xl border bg-card p-4", className)}>
      {title && (
        <header className="flex flex-col gap-0.5">
          <h3 className="text-sm font-semibold">{title}</h3>
          {description && <p className="text-[13px] text-muted-foreground">{description}</p>}
        </header>
      )}
      {children}
    </section>
  )
}

/** Who the agent is at a glance: avatar, name, model, what it's doing; open its chat or computer. */
function AgentHero({ agent, onClose }: { agent: Agent; onClose: () => void }) {
  const navigate = useNavigate()
  const dm = useAgentDm(agent.id)
  const working = activityLabel(agent)
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3.5">
        <AgentAvatar
          id={agent.id}
          name={agent.name}
          size="lg"
          active={agent.activity.state === "working"}
          className="size-12"
        />
        <div className="flex min-w-0 flex-col gap-1">
          <SheetTitle className="truncate text-lg leading-6 font-semibold">{agent.name}</SheetTitle>
          <SheetDescription className="flex min-w-0 items-center gap-2 text-[13px]">
            <span className="truncate rounded-md bg-muted px-1.5 font-mono text-xs text-muted-foreground">
              {agent.model}
            </span>
            {working ? (
              <span className="shimmer truncate">{working}</span>
            ) : (
              <span className="flex items-center gap-1.5">
                <span className="size-1.5 rounded-full bg-success" aria-hidden="true" />
                Idle
              </span>
            )}
            {agent.trustMode === "trusted" && (
              <span className="flex items-center gap-1 text-warning-foreground">
                <ShieldCheckIcon className="size-3.5" aria-hidden="true" />
                Trusted
              </span>
            )}
          </SheetDescription>
        </div>
      </div>
      <div className="flex gap-2">
        {dm && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              onClose()
              void navigate({ to: "/chats/$chatId", params: { chatId: dm.id } })
            }}
          >
            <MessageSquareIcon />
            Open chat
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            onClose()
            void navigate({ to: "/agents/$agentId/computer", params: { agentId: agent.id } })
          }}
        >
          <MonitorIcon />
          Computer
        </Button>
      </div>
    </div>
  )
}

/** The sheet's tabs, with how many memories and tasks the agent has. */
function AgentTabs({
  agent,
  tab,
  onChange,
}: {
  agent: Agent
  tab: Tab
  onChange: (tab: Tab) => void
}) {
  const memories = useMemories(agent.id)
  const tasks = useTasks(agent.id)
  return (
    <SegmentedControl
      label={`${agent.name}'s settings`}
      value={tab}
      onChange={onChange}
      className="max-w-full [scrollbar-width:none] overflow-x-auto"
      options={[
        { value: "profile", label: "Profile" },
        { value: "behavior", label: "Behavior" },
        { value: "memory", label: "Memory", count: memories.data?.length },
        { value: "tasks", label: "Tasks", count: tasks.data?.length },
        { value: "access", label: "Access" },
        { value: "usage", label: "Usage" },
      ]}
    />
  )
}

function TabBody({
  tab,
  agent,
  modelInputRef,
  onClose,
}: {
  tab: Tab
  agent: Agent
  modelInputRef: Ref<HTMLInputElement>
  onClose: () => void
}) {
  switch (tab) {
    case "profile":
      return (
        <>
          <Card title="Identity">
            <FieldGroup>
              <NameSection agent={agent} />
              <ModelField agent={agent} inputRef={modelInputRef} />
            </FieldGroup>
          </Card>
          <Card title="How it works" description="What it's for, and how it comes across.">
            <FieldGroup>
              <InstructionsSection agent={agent} />
              <PersonalitySection agent={agent} />
              <LanguageSection agent={agent} />
            </FieldGroup>
          </Card>
          <DangerZone agent={agent} />
        </>
      )
    case "behavior":
      return (
        <>
          <Card title="Permissions & alerts">
            <FieldGroup className="gap-5">
              <NotificationsSection agent={agent} />
              <TrustSection agent={agent} />
            </FieldGroup>
          </Card>
          <Card>
            <StandingApprovalsSection agent={agent} />
          </Card>
        </>
      )
    case "memory":
      return (
        <Card>
          <MemoriesSection agent={agent} />
        </Card>
      )
    case "tasks":
      return (
        <Card>
          <TasksSection agent={agent} />
        </Card>
      )
    case "access":
      return (
        <>
          <Card>
            <AgentConnections agentId={agent.id} agentName={agent.name} onNavigate={onClose} />
          </Card>
          <Card>
            <SecretsSection agent={agent} />
          </Card>
          <Card>
            <ComputerSection agent={agent} onOpen={onClose} />
          </Card>
        </>
      )
    case "usage":
      return (
        <Card>
          <AgentUsageSection agent={agent} />
        </Card>
      )
    default:
      return null
  }
}

/**
 * Agent details in a side sheet, opened from the chat header or a failure notice (see
 * `openAgentDetails`). Mounted once. A header with who the agent is, then tabs of settings
 * grouped in cards.
 */
export function AgentDetailsSheet() {
  const mobile = useIsMobile()
  const agentId = useAgentDetailsStore((s) => s.agentId)
  const focus = useAgentDetailsStore((s) => s.focus)
  const close = useAgentDetailsStore((s) => s.close)
  const agent = useAgentsById().get(agentId ?? "")
  // Keep showing the last agent while the sheet animates closed.
  const [shown, setShown] = useState<Agent | undefined>(agent)
  if (agent && agent !== shown) setShown(agent)
  const modelInputRef = useRef<HTMLInputElement>(null)
  const [tab, setTab] = useState<Tab>("profile")
  // Each opening starts on Profile (where "Change model" points).
  const [openedFor, setOpenedFor] = useState(agentId)
  if (agentId !== openedFor) {
    setOpenedFor(agentId)
    if (agentId) setTab("profile")
  }

  return (
    <Sheet
      open={agentId !== null && agent !== undefined}
      onOpenChange={(open) => {
        if (!open) close()
      }}
    >
      {shown && (
        <SheetContent
          side="right"
          // On phones it's a full screen pushed from the right, with a back button.
          className="gap-0 data-[side=right]:w-full max-md:shadow-none max-md:data-ending-style:translate-x-full max-md:data-starting-style:translate-x-full data-[side=right]:sm:max-w-xl"
          showCloseButton={!mobile}
          initialFocus={focus === "model" ? modelInputRef : undefined}
        >
          <SheetHeader
            className={cn(
              "gap-4 border-b pb-3",
              mobile ? "pt-[max(1rem,env(safe-area-inset-top))]" : "pt-5 pr-12",
            )}
          >
            {mobile && (
              <Button variant="ghost" size="sm" className="-ml-2 w-fit" onClick={close}>
                <ChevronLeftIcon />
                Back
              </Button>
            )}
            <AgentHero agent={shown} onClose={close} />
            <AgentTabs agent={shown} tab={tab} onChange={setTab} />
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-background/40">
            {/* Keyed by agent so switching agents never shows another agent's drafts. */}
            <div key={shown.id} className="flex flex-col gap-4 p-4 pb-8">
              <TabBody tab={tab} agent={shown} modelInputRef={modelInputRef} onClose={close} />
            </div>
          </div>
        </SheetContent>
      )}
    </Sheet>
  )
}

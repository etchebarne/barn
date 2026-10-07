import { useState } from "react"

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

import { AddConnection, type AddStep } from "./add-connection"
import { useConnectors, useConnectorTypes } from "./api"
import { ConnectorDetail } from "./connector-detail"
import { ConnectorIcon } from "./connector-icon"

const ADD_TITLES: Record<AddStep["step"], { title: string; description: string }> = {
  type: { title: "Add connection", description: "Pick an app to connect." },
  form: {
    title: "Add connection",
    description: "barn checks the details with the app before saving.",
  },
  done: { title: "Connected", description: "Agents you picked can use it now." },
}

/**
 * The one connectors sheet: the add flow ("new") or a connection's detail (its id). Content
 * changes in place, so dialogs never stack.
 */
export function ConnectorSheet({
  selection,
  onSelect,
}: {
  selection: string | undefined
  onSelect: (selection: string | undefined) => void
}) {
  const { data: connectors } = useConnectors()
  const { data: types } = useConnectorTypes()
  const [addStep, setAddStep] = useState<AddStep["step"]>("type")
  const connector = connectors?.find((c) => c.id === selection)
  const type = types?.find((t) => t.type === connector?.type)
  const adding = selection === "new"
  const open = adding || connector !== undefined
  function close() {
    setAddStep("type")
    onSelect(undefined)
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) close()
      }}
    >
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md"
      >
        {adding ? (
          <SheetHeader className="border-b pr-12">
            <SheetTitle>{ADD_TITLES[addStep].title}</SheetTitle>
            <SheetDescription>{ADD_TITLES[addStep].description}</SheetDescription>
          </SheetHeader>
        ) : connector ? (
          <SheetHeader className="flex-row items-center gap-3 border-b pr-12">
            <ConnectorIcon type={connector.type} className="size-10" />
            <div className="flex min-w-0 flex-col gap-0.5">
              <SheetTitle className="truncate">{connector.name}</SheetTitle>
              <SheetDescription>{type?.name ?? connector.type}</SheetDescription>
            </div>
          </SheetHeader>
        ) : null}
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
          {adding ? (
            <AddConnection
              key="add"
              onStepChange={setAddStep}
              onDone={close}
              onOpen={(id) => {
                setAddStep("type")
                onSelect(id)
              }}
            />
          ) : connector ? (
            <ConnectorDetail
              key={connector.id}
              connector={connector}
              type={type}
              onDisconnected={close}
            />
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  )
}

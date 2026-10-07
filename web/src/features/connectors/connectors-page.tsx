import { PageHeader } from "@/components/page-header"

import { ConnectorsSection } from "./connectors-section"

/** Connectors: the apps agents can use, with the add flow and each connection's details. */
export function ConnectorsPage({
  connector,
  onConnectorChange,
}: {
  /** The connection shown in the sheet ("new" for the add flow). */
  connector: string | undefined
  onConnectorChange: (connector: string | undefined) => void
}) {
  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="truncate text-sm font-medium">Connectors</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-4 py-2 md:px-6">
          <ConnectorsSection selection={connector} onSelect={onConnectorChange} />
        </div>
      </div>
    </div>
  )
}

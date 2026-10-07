import { PageHeader } from "@/components/page-header"

/** Shown at `/` when there are no chats yet. */
export function ChatsEmptyState() {
  return (
    <div className="flex h-svh flex-1 flex-col">
      <PageHeader>
        <h1 className="text-sm font-medium">openbot</h1>
      </PageHeader>
      <p className="m-auto p-6 text-center text-sm text-muted-foreground">No chats yet.</p>
    </div>
  )
}

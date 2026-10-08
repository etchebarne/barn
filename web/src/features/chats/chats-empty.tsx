import { useQuery } from "@tanstack/react-query"

import { PageHeader } from "@/components/page-header"

import { chatsQueryOptions } from "./api"

/** Shown at `/` on desktop when no chat is open (normally `/` opens the first chat). */
export function ChatsEmptyState() {
  const { data: chats } = useQuery(chatsQueryOptions)
  return (
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <PageHeader>
        <h1 className="text-sm font-medium">openbot</h1>
      </PageHeader>
      <p className="m-auto p-6 text-center text-sm text-muted-foreground">
        {chats && chats.length > 0 ? "Pick a chat on the left." : "No chats yet."}
      </p>
    </div>
  )
}

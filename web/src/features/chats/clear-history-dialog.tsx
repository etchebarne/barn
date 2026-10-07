import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"
import type { Chat } from "@/lib/api-client"
import { clearHistoryCopy, useClearChatHistory } from "@/lib/chat-history"

/** Confirms clearing a DM's history. Stays open (with a spinner) until the request settles. */
export function ClearHistoryDialog({
  chat,
  open,
  onOpenChange,
}: {
  chat: Pick<Chat, "id" | "name">
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const clear = useClearChatHistory(chat)
  const copy = clearHistoryCopy(chat.name)
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (clear.isPending) return
        onOpenChange(next)
      }}
    >
      <DialogContent showCloseButton={false} role="alertdialog">
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{copy.body}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            disabled={clear.isPending}
            autoFocus
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={clear.isPending}
            onClick={() => clear.mutate(undefined, { onSuccess: () => onOpenChange(false) })}
          >
            {clear.isPending && <Spinner />}
            Clear history
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

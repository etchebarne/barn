import { FileIcon, PencilIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Markdown } from "@/features/chats"

import { useDeleteSkill, useSkill, type Skill } from "./api"
import { SkillForm } from "./skill-form"

function SkillDetail({
  skill,
  onEdit,
  onDeleted,
}: {
  skill: Skill
  onEdit: () => void
  onDeleted: () => void
}) {
  const remove = useDeleteSkill()
  // Deleting asks once, inline (no dialog over the sheet).
  const [confirming, setConfirming] = useState(false)
  const files = skill.files ?? []

  return (
    <div className="flex flex-col gap-6">
      {skill.problem && (
        <p className="flex gap-2 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">
          <TriangleAlertIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
          Agents can't use this skill: {skill.problem}.
        </p>
      )}
      <section className="flex flex-col gap-1">
        <h3 className="text-xs font-medium text-muted-foreground">When to use it</h3>
        <p className="text-sm whitespace-pre-line">{skill.description || "—"}</p>
      </section>
      <section className="flex flex-col gap-1">
        <h3 className="text-xs font-medium text-muted-foreground">Instructions</h3>
        <div className="rounded-lg border p-3 text-sm">
          <Markdown>{skill.instructions || "(empty)"}</Markdown>
        </div>
      </section>
      {files.length > 0 && (
        <section className="flex flex-col gap-1">
          <h3 className="text-xs font-medium text-muted-foreground">Files</h3>
          <ul className="flex flex-col gap-1">
            {files.map((file) => (
              <li key={file} className="flex items-center gap-2 text-sm">
                <FileIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                <code className="truncate font-mono text-xs">{file}</code>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            Edit them from any agent's computer, in {skill.path}.
          </p>
        </section>
      )}
      <div className="flex justify-end gap-2">
        {confirming ? (
          <>
            <Button variant="ghost" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              autoFocus
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(skill.name, {
                  onSuccess: () => {
                    toast.success(`Deleted the ${skill.name} skill`)
                    onDeleted()
                  },
                  onError: (error) => toast.error(`Couldn't delete it: ${error.message}`),
                })
              }
            >
              Delete skill and its files
            </Button>
          </>
        ) : (
          <>
            <Button
              variant="ghost"
              className="text-muted-foreground hover:text-destructive"
              onClick={() => setConfirming(true)}
            >
              <Trash2Icon />
              Delete
            </Button>
            {!skill.problem?.includes("frontmatter") && (
              <Button variant="outline" onClick={onEdit}>
                <PencilIcon />
                Edit
              </Button>
            )}
          </>
        )}
      </div>
    </div>
  )
}

/**
 * The one skills sheet: a skill's details, editing it, or writing a new one ("new"). Content
 * changes in place, so sheets never stack.
 */
export function SkillSheet({
  selection,
  onSelect,
}: {
  selection: string | undefined
  onSelect: (selection: string | undefined) => void
}) {
  const creating = selection === "new"
  const { data: skill, isPending, error } = useSkill(creating ? undefined : selection)
  const [editing, setEditing] = useState<string | null>(null)
  const isEditing = !creating && editing !== null && editing === selection

  return (
    <Sheet
      open={selection !== undefined}
      onOpenChange={(open) => {
        if (!open) {
          setEditing(null)
          onSelect(undefined)
        }
      }}
    >
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg"
      >
        <SheetHeader className="border-b pr-12">
          <SheetTitle className="truncate">{creating ? "New skill" : selection}</SheetTitle>
          <SheetDescription>
            {creating
              ? "Instructions any agent can follow for a kind of job."
              : isEditing
                ? "Agents use the new version from their next turn."
                : skill?.path}
          </SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
          {creating ? (
            <SkillForm
              skill={undefined}
              onSaved={(name) => onSelect(name)}
              onCancel={() => onSelect(undefined)}
            />
          ) : isPending ? (
            <div className="flex flex-col gap-3">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-32" />
            </div>
          ) : error ? (
            <p className="text-sm text-destructive">Couldn't load it: {error.message}</p>
          ) : isEditing ? (
            <SkillForm
              skill={skill}
              onSaved={() => setEditing(null)}
              onCancel={() => setEditing(null)}
            />
          ) : (
            <SkillDetail
              skill={skill}
              onEdit={() => setEditing(skill.name)}
              onDeleted={() => onSelect(undefined)}
            />
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

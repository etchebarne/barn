import { useId, useState } from "react"

import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"

import { slugify, useSaveSkill, type Skill } from "./api"

/**
 * Writing a skill: its name (new skills only; it names the folder), when to use it, and the
 * instructions. The server checks the fields and its message shows under the form.
 */
export function SkillForm({
  skill,
  onSaved,
  onCancel,
}: {
  /** The skill being edited; none for a new one. */
  skill: Skill | undefined
  onSaved: (name: string) => void
  onCancel: () => void
}) {
  const id = useId()
  const save = useSaveSkill()
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState(skill?.description ?? "")
  const [instructions, setInstructions] = useState(skill?.instructions ?? "")
  const name = skill?.name ?? slugify(title)
  const ready = name !== "" && description.trim() !== "" && instructions.trim() !== ""

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault()
        if (!ready) return
        save.mutate(
          { name, description, instructions },
          { onSuccess: (saved) => onSaved(saved.name) },
        )
      }}
    >
      {!skill && (
        <Field>
          <FieldLabel htmlFor={`${id}-name`}>Name</FieldLabel>
          <Input
            id={`${id}-name`}
            value={title}
            autoFocus
            maxLength={64}
            placeholder="Weekly report"
            onChange={(event) => setTitle(event.target.value)}
          />
          <FieldDescription>
            {name ? (
              <>
                Saved as <code className="font-mono text-foreground/80">/shared/skills/{name}</code>
              </>
            ) : (
              "Lowercase letters, digits and hyphens make the folder name."
            )}
          </FieldDescription>
        </Field>
      )}
      <Field>
        <FieldLabel htmlFor={`${id}-description`}>When to use it</FieldLabel>
        <Textarea
          id={`${id}-description`}
          value={description}
          maxLength={1024}
          rows={2}
          placeholder="Writes the Friday status report from the week's merged PRs. Use when asked for the weekly report."
          onChange={(event) => setDescription(event.target.value)}
        />
        <FieldDescription>
          Agents see this and decide from it whether the skill fits a job.
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${id}-instructions`}>Instructions</FieldLabel>
        <Textarea
          id={`${id}-instructions`}
          value={instructions}
          rows={10}
          className="max-h-[50svh] font-mono text-xs md:text-xs"
          placeholder={
            "1. List the PRs merged since last Friday.\n2. Group them by area.\n3. Post a short table in my DM."
          }
          onChange={(event) => setInstructions(event.target.value)}
        />
        <FieldDescription>
          Markdown. Refer to scripts or templates in the skill's folder by path.
        </FieldDescription>
      </Field>
      {save.error && <FieldError>{save.error.message}</FieldError>}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={!ready || save.isPending}>
          {save.isPending && <Spinner />}
          {skill ? "Save" : "Create skill"}
        </Button>
      </div>
    </form>
  )
}

import { ChevronRightIcon, PlusIcon, SparklesIcon, TriangleAlertIcon } from "lucide-react"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

import { useSkills } from "./api"
import { SkillSheet } from "./skill-sheet"

/**
 * Skills: the shared instructions agents follow for kinds of jobs, kept as folders in
 * /shared/skills. Agents read them when a job fits and propose new ones (which you approve);
 * here you can read, write, edit and delete them.
 */
export function SkillsPage({
  selection,
  onSelect,
}: {
  /** The skill shown in the sheet ("new" to write one). */
  selection: string | undefined
  onSelect: (selection: string | undefined) => void
}) {
  const { data: skills, isPending, error } = useSkills()

  return (
    <div className="flex h-svh min-w-0 flex-1 flex-col">
      <PageHeader
        actions={
          <Button size="sm" variant="outline" onClick={() => onSelect("new")}>
            <PlusIcon />
            New skill
          </Button>
        }
      >
        <h1 className="truncate text-sm font-medium">Skills</h1>
      </PageHeader>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 px-4 py-6 md:px-6">
          <p className="text-sm text-muted-foreground">
            Instructions any agent follows for a kind of job. Agents see each skill's name and when
            to use it, and read the rest when a job fits. They can propose new ones too; you approve
            each. Skills live in{" "}
            <code className="font-mono text-foreground/80">/shared/skills</code>, one folder each,
            in the same SKILL.md format Claude Code and Codex use, so you can copy skills in from
            elsewhere.
          </p>
          {isPending && (
            <div className="flex flex-col gap-2">
              <Skeleton className="h-14" />
              <Skeleton className="h-14" />
            </div>
          )}
          {error && (
            <p className="text-sm text-destructive">Couldn't load skills: {error.message}</p>
          )}
          {skills?.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-12 text-center">
              <SparklesIcon aria-hidden="true" className="size-8 text-muted-foreground/60" />
              <p className="text-sm font-medium">No skills yet</p>
              <p className="max-w-sm text-sm text-muted-foreground">
                Write one, or ask an agent to turn something it just did into a skill.
              </p>
            </div>
          )}
          {skills && skills.length > 0 && (
            <ul className="flex flex-col gap-1" aria-label="Skills">
              {skills.map((skill) => (
                <li key={skill.name}>
                  <button
                    type="button"
                    onClick={() => onSelect(skill.name)}
                    className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm outline-none select-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50"
                  >
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className="truncate font-medium">{skill.name}</span>
                      {skill.problem ? (
                        <span className="flex items-center gap-1 text-xs text-warning-foreground">
                          <TriangleAlertIcon aria-hidden="true" className="size-3 shrink-0" />
                          <span className="truncate">Can't be used: {skill.problem}</span>
                        </span>
                      ) : (
                        <span className="line-clamp-2 text-xs text-muted-foreground">
                          {skill.description}
                        </span>
                      )}
                    </span>
                    <ChevronRightIcon
                      aria-hidden="true"
                      className="size-4 shrink-0 text-muted-foreground/60"
                    />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
      <SkillSheet selection={selection} onSelect={onSelect} />
    </div>
  )
}

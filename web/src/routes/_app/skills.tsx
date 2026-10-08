import { createFileRoute } from "@tanstack/react-router"

import { SkillsPage } from "@/features/skills"

type SkillsSearch = { skill?: string }

export const Route = createFileRoute("/_app/skills")({
  validateSearch: (search: Record<string, unknown>): SkillsSearch =>
    typeof search.skill === "string" && search.skill ? { skill: search.skill } : {},
  component: SkillsRoute,
})

function SkillsRoute() {
  const navigate = Route.useNavigate()
  const { skill } = Route.useSearch()
  return (
    <SkillsPage
      selection={skill}
      onSelect={(next) => void navigate({ search: next ? { skill: next } : {}, replace: true })}
    />
  )
}

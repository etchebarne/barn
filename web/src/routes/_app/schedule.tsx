import { createFileRoute } from "@tanstack/react-router"

import { SchedulePage } from "@/features/schedule"

type ScheduleSearch = { task?: string }

export const Route = createFileRoute("/_app/schedule")({
  validateSearch: (search: Record<string, unknown>): ScheduleSearch =>
    typeof search.task === "string" && search.task ? { task: search.task } : {},
  component: ScheduleRoute,
})

function ScheduleRoute() {
  const navigate = Route.useNavigate()
  const { task } = Route.useSearch()
  return (
    <SchedulePage
      taskId={task}
      onTaskChange={(next) => void navigate({ search: next ? { task: next } : {}, replace: true })}
    />
  )
}

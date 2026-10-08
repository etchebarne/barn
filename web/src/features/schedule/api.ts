import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export type Schedule = Schemas["Schedule"]
export type TaskRun = Schemas["TaskRun"]
export type UpcomingRuns = Schemas["UpcomingRuns"]

/** Every agent's tasks, their next runs (a week ahead) and the latest runs. Live via `task.run`. */
export function useSchedule() {
  return useQuery({
    queryKey: queryKeys.schedule,
    queryFn: () => unwrap(api.GET("/schedule", { params: { query: { days: 7 } } })),
    // Agents change their tasks from chat, which sends no event: refresh when the page opens
    // and now and then while it's open.
    staleTime: 0,
    refetchOnMount: "always",
    refetchInterval: 60_000,
  })
}

/** A task's run history, newest first (unchanged checks included). */
export function useTaskRuns(taskId: string) {
  return useQuery({
    queryKey: queryKeys.taskRuns(taskId),
    queryFn: () => unwrap(api.GET("/tasks/{taskId}/runs", { params: { path: { taskId } } })),
    staleTime: 0,
  })
}

/** Runs a task now; the run shows up through `task.run` events. */
export function useRunTaskNow() {
  return useMutation({
    mutationFn: (taskId: string) =>
      unwrap(api.POST("/tasks/{taskId}/run", { params: { path: { taskId } } })),
    onError: (error) => toast.error(`Couldn't run it: ${error.message}`),
  })
}

/** Pauses or resumes a task. */
export function useSetTaskEnabled() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ taskId, enabled }: { taskId: string; enabled: boolean }) =>
      unwrap(api.PATCH("/tasks/{taskId}", { params: { path: { taskId } }, body: { enabled } })),
    onSuccess: (task) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.schedule })
      void queryClient.invalidateQueries({ queryKey: queryKeys.tasks(task.agentId) })
    },
    onError: (error) => toast.error(`Couldn't change it: ${error.message}`),
  })
}

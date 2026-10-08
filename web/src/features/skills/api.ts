import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

export type Skill = Schemas["Skill"]

/** The skill library. Agents and files in /shared can change it, so it refreshes on open. */
export function useSkills() {
  return useQuery({
    queryKey: queryKeys.skills,
    queryFn: () => unwrap(api.GET("/skills")),
    staleTime: 0,
    refetchOnMount: "always",
    refetchOnWindowFocus: true,
  })
}

/** One skill with its instructions and files. */
export function useSkill(name: string | undefined) {
  return useQuery({
    queryKey: queryKeys.skill(name ?? ""),
    queryFn: () => unwrap(api.GET("/skills/{name}", { params: { path: { name: name ?? "" } } })),
    enabled: name !== undefined,
    staleTime: 0,
  })
}

/** Creates or rewrites a skill; 400 with a readable message when a field isn't valid. */
export function useSaveSkill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      description,
      instructions,
    }: { name: string } & Schemas["SaveSkillRequest"]) =>
      unwrap(
        api.PUT("/skills/{name}", {
          params: { path: { name } },
          body: { description, instructions },
        }),
      ),
    onSuccess: (skill) => {
      queryClient.setQueryData(queryKeys.skill(skill.name), skill)
      void queryClient.invalidateQueries({ queryKey: queryKeys.skills, exact: true })
    },
  })
}

export function useDeleteSkill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      unwrap(api.DELETE("/skills/{name}", { params: { path: { name } } })),
    onSuccess: (_data, name) => {
      queryClient.setQueryData<Skill[]>(queryKeys.skills, (list) =>
        list?.filter((s) => s.name !== name),
      )
      queryClient.removeQueries({ queryKey: queryKeys.skill(name) })
    },
  })
}

/** A name from what someone typed: "Weekly Report!" → "weekly-report". */
export function slugify(text: string): string {
  return text
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64)
    .replace(/-+$/, "")
}

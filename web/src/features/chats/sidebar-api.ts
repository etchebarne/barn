import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, unwrap, type Chat } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

import {
  applyLayoutToChats,
  orderCategories,
  type SidebarCategory,
  type SidebarLayout,
} from "./sidebar-layout"

export const categoriesQueryOptions = queryOptions({
  queryKey: queryKeys.sidebarCategories,
  queryFn: () => unwrap(api.GET("/sidebar/categories")),
})

export function useCategories() {
  return useQuery(categoriesQueryOptions)
}

export function useCreateCategory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => unwrap(api.POST("/sidebar/categories", { body: { name } })),
    onSuccess: (category) =>
      queryClient.setQueryData<SidebarCategory[]>(queryKeys.sidebarCategories, (list) => [
        ...(list ?? []),
        category,
      ]),
    onError: (error) => toast.error(`Couldn't create the category: ${error.message}`),
  })
}

/** Rename or collapse a category (optimistic). */
export function useUpdateCategory() {
  const queryClient = useQueryClient()
  const key = queryKeys.sidebarCategories
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; collapsed?: boolean }) =>
      unwrap(
        api.PATCH("/sidebar/categories/{categoryId}", {
          params: { path: { categoryId: id } },
          body,
        }),
      ),
    onMutate: ({ id, ...change }) => {
      const previous = queryClient.getQueryData<SidebarCategory[]>(key)
      queryClient.setQueryData<SidebarCategory[]>(key, (list) =>
        list?.map((c) => (c.id === id ? Object.assign({}, c, change) : c)),
      )
      return { previous }
    },
    onError: (error, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(key, context.previous)
      toast.error(`Couldn't update the category: ${error.message}`)
    },
  })
}

/** Deletes a category; its chats move to Unassigned (optimistic). */
export function useDeleteCategory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(
        api.DELETE("/sidebar/categories/{categoryId}", { params: { path: { categoryId: id } } }),
      ),
    onMutate: (id) => {
      const categories = queryClient.getQueryData<SidebarCategory[]>(queryKeys.sidebarCategories)
      const chats = queryClient.getQueryData<Chat[]>(queryKeys.chats)
      queryClient.setQueryData<SidebarCategory[]>(queryKeys.sidebarCategories, (list) =>
        list?.filter((c) => c.id !== id),
      )
      queryClient.setQueryData<Chat[]>(queryKeys.chats, (list) =>
        list?.map((c) =>
          c.categoryId === id ? Object.assign({}, c, { categoryId: null, position: null }) : c,
        ),
      )
      return { categories, chats }
    },
    onError: (error, _id, context) => {
      if (context?.categories)
        queryClient.setQueryData(queryKeys.sidebarCategories, context.categories)
      if (context?.chats) queryClient.setQueryData(queryKeys.chats, context.chats)
      toast.error(`Couldn't delete the category: ${error.message}`)
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.chats, exact: true }),
  })
}

/** Saves a new order/placement: applied right away, rolled back with a toast if it fails. */
export function useSaveLayout() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (layout: SidebarLayout) => unwrap(api.PUT("/sidebar/layout", { body: layout })),
    onMutate: (layout) => {
      const categories = queryClient.getQueryData<SidebarCategory[]>(queryKeys.sidebarCategories)
      const chats = queryClient.getQueryData<Chat[]>(queryKeys.chats)
      queryClient.setQueryData<SidebarCategory[]>(queryKeys.sidebarCategories, (list) =>
        list ? orderCategories(list, layout.categoryOrder) : list,
      )
      queryClient.setQueryData<Chat[]>(queryKeys.chats, (list) =>
        list ? applyLayoutToChats(list, layout) : list,
      )
      return { categories, chats }
    },
    onError: (error, _layout, context) => {
      if (context?.categories)
        queryClient.setQueryData(queryKeys.sidebarCategories, context.categories)
      if (context?.chats) queryClient.setQueryData(queryKeys.chats, context.chats)
      toast.error(`Couldn't save the sidebar: ${error.message}`)
    },
  })
}

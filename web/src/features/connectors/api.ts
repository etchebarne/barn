import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Schemas } from "@/lib/api-client"
import { queryKeys } from "@/lib/query-keys"

import type { Connector } from "./logic"

export const connectorTypesQueryOptions = queryOptions({
  queryKey: queryKeys.connectorTypes,
  queryFn: () => unwrap(api.GET("/connectors/types")),
  staleTime: 10 * 60_000,
})

export const connectorsQueryOptions = queryOptions({
  queryKey: queryKeys.connectors,
  queryFn: () => unwrap(api.GET("/connectors")),
})

export function useConnectors() {
  return useQuery(connectorsQueryOptions)
}

export function useConnectorTypes() {
  return useQuery(connectorTypesQueryOptions)
}

function replace(list: Connector[] | undefined, connector: Connector) {
  return list?.map((c) => (c.id === connector.id ? connector : c))
}

/** Creates a connection. 400/502: the service rejected the credentials (readable message). */
export function useCreateConnector() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: Schemas["CreateConnectorRequest"]) =>
      unwrap(api.POST("/connectors", { body })),
    onSuccess: (connector) =>
      queryClient.setQueryData<Connector[]>(queryKeys.connectors, (list) =>
        list ? [...list, connector] : [connector],
      ),
  })
}

export function useUpdateConnector(connectorId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: Schemas["UpdateConnectorRequest"]) =>
      unwrap(api.PATCH("/connectors/{connectorId}", { params: { path: { connectorId } }, body })),
    onSuccess: (connector) =>
      queryClient.setQueryData<Connector[]>(queryKeys.connectors, (list) =>
        replace(list, connector),
      ),
  })
}

/** Agent access, saved immediately (optimistic). */
export function useSetConnectorAgents(connectorId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (agentIds: string[]) =>
      unwrap(
        api.PATCH("/connectors/{connectorId}", {
          params: { path: { connectorId } },
          body: { agentIds },
        }),
      ),
    onMutate: (agentIds) => {
      const previous = queryClient.getQueryData<Connector[]>(queryKeys.connectors)
      queryClient.setQueryData<Connector[]>(queryKeys.connectors, (list) =>
        list?.map((c) => (c.id === connectorId ? { ...c, agentIds } : c)),
      )
      return { previous }
    },
    onSuccess: (connector) =>
      queryClient.setQueryData<Connector[]>(queryKeys.connectors, (list) =>
        replace(list, connector),
      ),
    onError: (_error, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(queryKeys.connectors, context.previous)
    },
  })
}

export function useDeleteConnector(connectorId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      unwrap(api.DELETE("/connectors/{connectorId}", { params: { path: { connectorId } } })),
    onSuccess: () => {
      queryClient.setQueryData<Connector[]>(queryKeys.connectors, (list) =>
        list?.filter((c) => c.id !== connectorId),
      )
      // The server drops tasks that waited on this connection's events.
      void queryClient.invalidateQueries({
        predicate: (q) => q.queryKey[0] === "agents" && q.queryKey[2] === "tasks",
      })
    },
  })
}

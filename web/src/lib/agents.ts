import { queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "./api-client"
import { queryKeys } from "./query-keys"

/** All agents (kept fresh by the WebSocket). Shared so any feature can read agent names. */
export const agentsQueryOptions = queryOptions({
  queryKey: queryKeys.agents,
  queryFn: () => unwrap(api.GET("/agents")),
})

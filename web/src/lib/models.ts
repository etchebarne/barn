import { queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "./api-client"
import { queryKeys } from "./query-keys"

/** Models available from the provider (needs a configured API key). */
export const modelsQueryOptions = queryOptions({
  queryKey: queryKeys.models,
  queryFn: () => unwrap(api.GET("/models")),
  staleTime: 5 * 60_000,
})

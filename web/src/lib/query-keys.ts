/**
 * Query keys shared between features and the realtime layer, so WebSocket events can
 * update exactly the caches the UI reads.
 */
export const queryKeys = {
  authStatus: ["auth", "status"] as const,
  onboarding: ["onboarding"] as const,
  providerSettings: ["settings", "provider"] as const,
  models: ["models"] as const,
  agents: ["agents"] as const,
  memories: (agentId: string) => ["agents", agentId, "memories"] as const,
  tasks: (agentId: string) => ["agents", agentId, "tasks"] as const,
  chats: ["chats"] as const,
  connectorTypes: ["connectors", "types"] as const,
  connectors: ["connectors", "list"] as const,
  connectorCatalog: ["connectors", "catalog"] as const,
  messages: (chatId: string) => ["chats", chatId, "messages"] as const,
}

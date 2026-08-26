export const RESOURCE_MANAGER_OPEN_EVENT = "neo-chat:open-resource-manager";

export interface ResourceManagerOpenDetail {
  kind: "skill" | "mcp";
  query: string;
  resourceId?: string;
}

import type { McpMarketplaceItem } from "@/lib/mcp/types";

export function appendUniqueMarketplaceItems(
  current: McpMarketplaceItem[],
  incoming: McpMarketplaceItem[],
): McpMarketplaceItem[] {
  const identifiers = new Set(current.map((item) => item.identifier));
  const appended = [...current];
  for (const item of incoming) {
    if (identifiers.has(item.identifier)) continue;
    identifiers.add(item.identifier);
    appended.push(item);
  }
  return appended;
}

export function hasNextMarketplacePage(
  page: number,
  totalPages: number,
): boolean {
  return page > 0 && totalPages > page;
}

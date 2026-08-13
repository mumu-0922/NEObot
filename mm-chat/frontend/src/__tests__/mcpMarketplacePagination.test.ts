import { describe, expect, it } from "vitest";

import en from "../i18n/locales/en";
import ja from "../i18n/locales/ja";
import zh from "../i18n/locales/zh";

import type { McpMarketplaceItem } from "../lib/mcp/types";
import {
  appendUniqueMarketplaceItems,
  hasNextMarketplacePage,
} from "../lib/mcp/marketplacePagination";

function item(identifier: string): McpMarketplaceItem {
  return {
    identifier,
    name: identifier,
    description: "",
    toolCount: 0,
    installCount: 0,
    stars: 0,
    rating: 0,
    official: false,
    validated: false,
  };
}

describe("MCP Marketplace pagination", () => {
  it("appends pages in order without duplicate identifiers", () => {
    const first = [item("one"), item("two")];
    const merged = appendUniqueMarketplaceItems(first, [
      item("two"),
      item("three"),
      item("three"),
    ]);

    expect(merged.map((entry) => entry.identifier)).toEqual([
      "one",
      "two",
      "three",
    ]);
    expect(first.map((entry) => entry.identifier)).toEqual(["one", "two"]);
  });

  it("stops at an empty, invalid, or final page", () => {
    expect(hasNextMarketplacePage(0, 10)).toBe(false);
    expect(hasNextMarketplacePage(1, 3)).toBe(true);
    expect(hasNextMarketplacePage(3, 3)).toBe(false);
    expect(hasNextMarketplacePage(4, 3)).toBe(false);
  });

  it("ships pagination copy in every supported locale", () => {
    for (const messages of [en, ja, zh]) {
      expect(messages.Mcp.marketplaceResults).toContain("{loaded}");
      expect(messages.Mcp.marketplaceResults).toContain("{count}");
      expect(messages.Mcp.marketplaceLoadMore).toBeTruthy();
      expect(messages.Mcp.marketplaceLoadingMore).toBeTruthy();
      expect(messages.Mcp.marketplaceRetry).toBeTruthy();
      expect(messages.Mcp.marketplaceAllLoaded).toContain("{count}");
    }
  });
});

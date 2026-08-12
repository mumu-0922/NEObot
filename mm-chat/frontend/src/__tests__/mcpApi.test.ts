import { describe, expect, it } from "vitest";

import { createServerMcpApiShell } from "../services/api/client/server/mcpApi";
import type { HttpClient } from "../services/api/client/server/httpClient";

describe("server MCP API", () => {
  it("uses the strict MCP product routes and normalizes responses", async () => {
    const requests: Array<{ path: string; method?: string; body?: unknown }> =
      [];
    const httpClient = {
      async requestJson(
        path: string,
        options: { method?: string; body?: unknown } = {},
      ) {
        requests.push({ path, method: options.method, body: options.body });
        if (path.includes("/marketplace/search")) {
          return {
            items: [
              {
                identifier: "deepwiki",
                name: "DeepWiki",
                description: "Repository docs",
                toolCount: 3,
                installCount: 10,
                stars: 20,
                rating: 4.8,
                official: true,
                validated: true,
              },
            ],
            categories: [{ category: "developer", count: 42 }],
            page: 1,
            pageSize: 20,
            totalCount: 1,
            totalPages: 1,
            source: "lobehub",
            sourceUrl: "https://market.lobehub.com",
          };
        }
        if (path.endsWith("/marketplace/items/deepwiki/install")) {
          return {
            server: {
              ref: { source: "private", id: "server-1" },
              name: "DeepWiki",
              icon: "https://github.com/deepwiki.png",
              transport: "streamable_http",
              authType: "none",
              status: "ready",
              hasCredential: true,
              toolCount: 0,
              unsupportedToolCount: 0,
              grants: [],
              tools: [],
            },
            enabledForConversation: false,
          };
        }
        if (path.includes("/marketplace/items/deepwiki")) {
          return {
            item: {
              identifier: "deepwiki",
              name: "DeepWiki",
              description: "Repository docs",
              toolCount: 3,
              installCount: 10,
              stars: 20,
              rating: 4.8,
              official: true,
              validated: true,
              version: "1.2.3",
              source: "lobehub",
              sourceUrl: "https://market.lobehub.com",
              tools: [],
              deployments: [
                {
                  connectionType: "http",
                  installationMethod: "none",
                  recommended: true,
                  compatibility: "installable",
                  compatibilityReason: "Public HTTPS Streamable HTTP",
                  endpointUrl: "https://mcp.deepwiki.com/mcp",
                  hash: "a".repeat(64),
                },
              ],
            },
          };
        }
        if (path.endsWith("/selection") && options.method === "PUT") {
          return {
            selection: {
              conversationId: "conversation-1",
              mode: "custom",
              revision: 2,
              servers: [],
            },
          };
        }
        return {
          servers: [
            {
              ref: { source: "catalog", id: "weather" },
              name: "Weather",
              transport: "streamable_http",
              authType: "none",
              status: "ready",
              hasCredential: true,
              toolCount: 0,
              unsupportedToolCount: 0,
              grants: [],
              tools: [],
            },
          ],
        };
      },
    } as HttpClient;
    const api = createServerMcpApiShell(httpClient);

    await expect(
      api.listServers({ conversationId: "conversation-1" }),
    ).resolves.toEqual([
      expect.objectContaining({ name: "Weather", status: "ready" }),
    ]);
    await expect(
      api.replaceConversationSelection({
        conversationId: "conversation-1",
        mode: "custom",
        revision: 1,
        servers: [],
      }),
    ).resolves.toMatchObject({ mode: "custom", revision: 2, servers: [] });
    await expect(
      api.searchMarketplace({ query: "deep wiki", category: "developer" }),
    ).resolves.toMatchObject({
      items: [expect.objectContaining({ identifier: "deepwiki" })],
      categories: [{ category: "developer", count: 42 }],
      source: "lobehub",
    });
    await expect(
      api.getMarketplaceItem({ identifier: "deepwiki", version: "1.2.3" }),
    ).resolves.toMatchObject({ version: "1.2.3" });
    await expect(
      api.installMarketplaceItem({
        identifier: "deepwiki",
        version: "1.2.3",
        selectionRevision: 0,
        enableForConversation: false,
      }),
    ).resolves.toMatchObject({
      server: { name: "DeepWiki", icon: "https://github.com/deepwiki.png" },
      enabledForConversation: false,
    });

    expect(requests).toEqual([
      {
        path: "/v1/mcp/servers?conversationId=conversation-1",
        method: undefined,
        body: undefined,
      },
      {
        path: "/v1/mcp/conversations/conversation-1/selection",
        method: "PUT",
        body: { mode: "custom", revision: 1, servers: [] },
      },
      {
        path: "/v1/mcp/marketplace/search?q=deep+wiki&category=developer&page=1&pageSize=20",
        method: undefined,
        body: undefined,
      },
      {
        path: "/v1/mcp/marketplace/items/deepwiki?version=1.2.3",
        method: undefined,
        body: undefined,
      },
      {
        path: "/v1/mcp/marketplace/items/deepwiki/install",
        method: "POST",
        body: {
          version: "1.2.3",
          selectionRevision: 0,
          enableForConversation: false,
        },
      },
    ]);
  });
});

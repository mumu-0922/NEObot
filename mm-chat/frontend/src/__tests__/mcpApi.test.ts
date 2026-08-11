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
    ]);
  });
});

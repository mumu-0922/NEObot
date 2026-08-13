import { describe, expect, it } from "vitest";

import {
  normalizeMcpCalls,
  normalizeMcpConversationSelectionEnvelope,
  normalizeMcpMarketplaceItemEnvelope,
  normalizeMcpMarketplaceSearch,
  normalizeMcpServer,
  normalizeMcpToolCallUpdate,
} from "../lib/mcp/types";

describe("MCP runtime DTO normalization", () => {
  it("normalizes camelCase call records and keeps only bounded summaries", () => {
    expect(
      normalizeMcpCalls({
        calls: [
          {
            id: "call-1",
            conversationId: "conversation-1",
            messageId: "message-1",
            runId: "run-1",
            serverRef: { source: "manifest", id: "files" },
            toolName: "read_file",
            toolAlias: "files_read_file",
            classification: "read",
            status: "succeeded",
            round: 1,
            call: 1,
            argumentsSummary: { path: "string" },
            resultSummary: "text:12",
            errorCode: "",
            durationMillis: 18,
          },
        ],
      }),
    ).toEqual([
      expect.objectContaining({
        id: "call-1",
        status: "succeeded",
        argumentsSummary: { path: "string" },
        durationMillis: 18,
      }),
    ]);
  });

  it("distinguishes inherit from an explicit empty custom selection", () => {
    expect(
      normalizeMcpConversationSelectionEnvelope({
        selection: {
          conversationId: "conversation-1",
          mode: "custom",
          revision: 4,
          servers: [],
        },
      }),
    ).toEqual({
      conversationId: "conversation-1",
      mode: "custom",
      revision: 4,
      servers: [],
    });
  });

  it("keeps only normalized display icons on installed Server DTOs", () => {
    const server = {
      ref: { source: "private", id: "context7" },
      name: "Context7",
      transport: "stdio",
      authType: "none",
      status: "ready",
      hasCredential: false,
      toolCount: 0,
      unsupportedToolCount: 0,
      grants: [],
      tools: [],
    };

    expect(
      normalizeMcpServer({
        ...server,
        icon: "https://github.com/upstash.png",
      }),
    ).toMatchObject({ icon: "https://github.com/upstash.png" });
    expect(
      normalizeMcpServer({
        ...server,
        icon: "http://127.0.0.1/icon.png",
      }),
    ).not.toHaveProperty("icon");
  });

  it("strictly validates streamed MCP Tool call updates", () => {
    const update = normalizeMcpToolCallUpdate({
      executionId: "call-1",
      callId: "call-1",
      toolName: "read_file",
      server: "manifest:files",
      classification: "read",
      processStatus: "outcome_unknown",
      status: "outcome_unknown",
      round: 2,
      argumentsSummary: { path: "string" },
      failureCategory: "connection_lost",
      durationMillis: 30000,
      mode: "mcp",
    });
    expect(update).toMatchObject({
      processStatus: "outcome_unknown",
      status: "outcome_unknown",
      argumentsSummary: { path: "string" },
    });
    expect(
      normalizeMcpToolCallUpdate({
        ...update,
        classification: "trusted-because-server-said-so",
      }),
    ).toBeNull();
  });

  it("normalizes bounded Marketplace metadata and rejects unsafe installable URLs", () => {
    expect(
      normalizeMcpMarketplaceSearch({
        items: [
          {
            identifier: "deepwiki",
            name: "DeepWiki",
            description: "Docs",
            icon: "https://github.com/example.png",
            toolCount: 3,
            installCount: 1,
            stars: 2,
            rating: 5,
            official: false,
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
      }),
    ).toMatchObject({
      items: [
        {
          identifier: "deepwiki",
          icon: "https://github.com/example.png",
        },
      ],
      categories: [{ category: "developer", count: 42 }],
    });

    const item = {
      identifier: "deepwiki",
      name: "DeepWiki",
      description: "Docs",
      toolCount: 3,
      installCount: 1,
      stars: 2,
      rating: 5,
      official: false,
      validated: true,
      version: "1.0.0",
      installed: false,
      source: "lobehub",
      sourceUrl: "https://market.lobehub.com",
      tools: [],
      deployments: [
        {
          connectionType: "http",
          installationMethod: "none",
          recommended: true,
          compatibility: "installable",
          compatibilityReason: "Public HTTPS",
          endpointUrl: "http://127.0.0.1/mcp",
        },
      ],
    };
    expect(normalizeMcpMarketplaceItemEnvelope({ item })).toBeNull();
    expect(
      normalizeMcpMarketplaceItemEnvelope({
        item: {
          ...item,
          deployments: [
            {
              ...item.deployments[0],
              endpointUrl: "https://mcp.deepwiki.com/mcp",
            },
          ],
        },
      }),
    ).toMatchObject({ version: "1.0.0" });
    expect(
      normalizeMcpMarketplaceItemEnvelope({
        item: {
          ...item,
          deployments: [
            {
              connectionType: "stdio",
              installationMethod: "npm",
              recommended: true,
              compatibility: "installable",
              compatibilityReason: "Approved shared Runner artifact",
              hash: "a".repeat(64),
            },
          ],
        },
      }),
    ).toMatchObject({
      deployments: [
        {
          connectionType: "stdio",
          compatibility: "installable",
        },
      ],
    });
  });
});

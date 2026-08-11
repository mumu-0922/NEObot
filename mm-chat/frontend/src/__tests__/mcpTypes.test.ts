import { describe, expect, it } from "vitest";

import {
  normalizeMcpCalls,
  normalizeMcpConversationSelectionEnvelope,
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
});

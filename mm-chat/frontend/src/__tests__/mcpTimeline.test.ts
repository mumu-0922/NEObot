import { describe, expect, it } from "vitest";

import { createServerChatApiShell } from "../services/api/client/server/chatApi";
import type {
  HttpClient,
  JsonRequestOptions,
  SseRequestOptions,
} from "../services/api/client/server/httpClient";

describe("MCP chat preflight and timeline", () => {
  it("preflights before send with the selected provider identity", async () => {
    const requests: Array<{ path: string; body: unknown }> = [];
    const client = createServerChatApiShell(
      createHttpClient({
        requestJson: async (path, options) => {
          requests.push({ path, body: options?.body });
          return { enabled: true };
        },
      }),
    );

    await expect(
      client.preflightMcp({
        conversationId: "conversation-1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        provider: { id: "openai", type: "openai" },
      }),
    ).resolves.toEqual({ enabled: true });
    expect(requests).toEqual([
      {
        path: "/v1/chat/conversations/conversation-1/mcp-preflight",
        body: {
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          provider: { id: "openai", type: "openai" },
        },
      },
    ]);
  });

  it("strictly dispatches tool.call.updated between sequenced stream events", async () => {
    const updates: unknown[] = [];
    const client = createServerChatApiShell(
      createHttpClient({
        requestSse: async (_path, options) => {
          options.onFrame({
            event: "message.started",
            data: { type: "message.started", runId: "run-1", sequence: 1 },
          });
          options.onFrame({
            event: "tool.call.updated",
            data: {
              type: "tool.call.updated",
              runId: "run-1",
              sequence: 2,
              toolCall: {
                executionId: "local-skill-1-1",
                toolName: "file_write",
                classification: "write",
                processStatus: "running",
                status: "running",
                round: 1,
                argumentsSummary: { path: "string" },
                durationMillis: 0,
                mode: "local_direct",
              },
            },
          });
          options.onFrame({
            event: "message.completed",
            data: { type: "message.completed", runId: "run-1", sequence: 3 },
          });
        },
      }),
    );

    await expect(
      client.streamAssistantMessage(
        {
          conversationId: "conversation-1",
          userMessageId: "message-1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-1",
        },
        { onToolCall: (event) => updates.push(event.toolCall) },
      ),
    ).resolves.toMatchObject({ status: "completed" });
    expect(updates).toEqual([
      expect.objectContaining({
        callId: "local-skill-1-1",
        toolName: "file_write",
        classification: "write",
        argumentsSummary: { path: "string" },
      }),
    ]);
  });

  it("rejects malformed authoritative Agent events at the stream boundary", async () => {
    const client = createServerChatApiShell(
      createHttpClient({
        requestSse: async (_path, options) => {
          options.onFrame({
            event: "message.started",
            data: { type: "message.started", runId: "run-1", sequence: 1 },
          });
          options.onFrame({
            event: "agent.event",
            data: {
              type: "agent.event",
              runId: "run-1",
              sequence: 2,
              agentEvent: {
                eventId: "event-1",
                turnId: "turn-1",
                conversationId: "conversation-1",
                messageId: "message-1",
                runId: "run-1",
                sequence: 0,
                type: "tool.result",
                payload: {},
                occurredAt: "2026-08-20T12:00:00Z",
              },
            },
          });
        },
      }),
    );

    await expect(
      client.streamAssistantMessage({
        conversationId: "conversation-1",
        userMessageId: "message-1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-invalid-agent-event",
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: { code: "INVALID_SERVER_RESPONSE" },
    });
  });
});

function createHttpClient(overrides: {
  requestJson?: (
    path: string,
    options?: JsonRequestOptions,
  ) => Promise<unknown>;
  requestSse?: (path: string, options: SseRequestOptions) => Promise<void>;
}): HttpClient {
  return {
    buildUrl: (path) => path,
    requestJson: (overrides.requestJson ??
      (async () => {
        throw new Error("requestJson not mocked");
      })) as HttpClient["requestJson"],
    async requestMultipartJson() {
      throw new Error("requestMultipartJson not mocked");
    },
    async requestBinary() {
      throw new Error("requestBinary not mocked");
    },
    requestSse:
      overrides.requestSse ??
      (async () => {
        throw new Error("requestSse not mocked");
      }),
  };
}

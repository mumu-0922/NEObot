import { describe, expect, it } from "vitest";
import { createChatStreamService } from "../services/api/chatStreamService";
import type {
  ApiCapabilities,
  ChatApi,
  ChatMessageDTO,
  NeoChatApiClient,
  ResolvedApiClientConfig,
} from "../services/api/client";

const capabilities = {
  chatCrud: true,
  chatStream: true,
  files: false,
  auth: false,
  imports: false,
  rag: false,
  plugins: false,
  providerSettings: false,
} satisfies ApiCapabilities;

const resolvedConfig = {
  mode: "server",
  requestedMode: "server",
  baseUrl: "http://backend.test",
  networkEdge: "direct-cors",
  serverConfigured: true,
  warnings: [],
} satisfies ResolvedApiClientConfig;

const assistantMessageDto: ChatMessageDTO = {
  id: "m2",
  conversationId: "c1",
  role: "assistant",
  status: "completed",
  content: "hi",
  sequenceNo: 2,
  modelRef: { providerId: "openai", modelId: "gpt-5.5" },
  attachments: [
    {
      id: "a1",
      source: "server",
      fileId: "file-1",
      fileName: "report.pdf",
      mimeType: "application/pdf",
      size: 123,
      sha256: "abc",
      purpose: "input",
    },
  ],
  outputBlocks: [{ id: "block-1", type: "text", content: "hi" }],
  metadata: {},
  createdAt: "2026-07-08T00:00:02Z",
  updatedAt: "2026-07-08T00:00:02Z",
  completedAt: "2026-07-08T00:00:02Z",
};

describe("chat stream service", () => {
  it("delegates server streams and maps terminal messages", async () => {
    const calls: string[] = [];
    const service = createChatStreamService({
      client: createMockClient({
        async streamAssistantMessage(input, handlers) {
          calls.push(
            `stream:${input.conversationId}:${input.userMessageId}:${input.idempotencyKey}`,
          );
          handlers?.onDelta?.({
            type: "message.delta",
            runId: "run-1",
            delta: "hi",
          });
          return { status: "completed", message: assistantMessageDto };
        },
        async cancelRun(runId) {
          calls.push(`cancel:${runId}`);
          return { status: "cancelled", message: assistantMessageDto };
        },
      }),
    });
    const deltas: string[] = [];

    await expect(
      service.streamAssistantMessage(
        {
          conversationId: "c1",
          userMessageId: "m1",
          modelRef: { providerId: "openai", modelId: "gpt-5.5" },
          idempotencyKey: "stream-key",
        },
        {
          onDelta: (event) => deltas.push(String(event.delta)),
        },
      ),
    ).resolves.toMatchObject({
      status: "completed",
      message: {
        id: "m2",
        role: "model",
        content: "hi",
        model: "openai:gpt-5.5",
        attachments: [
          {
            id: "a1",
            url: "http://backend.test/v1/files/file-1/content",
          },
        ],
        outputBlocks: [{ id: "block-1", type: "text", content: "hi" }],
      },
    });
    await expect(service.cancelRun("run-1")).resolves.toMatchObject({
      status: "cancelled",
      message: { id: "m2", role: "model" },
    });
    expect(deltas).toEqual(["hi"]);
    expect(calls).toEqual(["stream:c1:m1:stream-key", "cancel:run-1"]);
  });

  it("fails closed when server stream is not enabled", async () => {
    let called = false;
    const service = createChatStreamService({
      client: createMockClient(
        {
          async streamAssistantMessage() {
            called = true;
            return { status: "completed" };
          },
        },
        { capabilities: { ...capabilities, chatStream: false } },
      ),
    });

    await expect(
      service.streamAssistantMessage({
        conversationId: "c1",
        userMessageId: "m1",
        modelRef: { providerId: "openai", modelId: "gpt-5.5" },
        idempotencyKey: "stream-key",
      }),
    ).rejects.toMatchObject({
      code: "SERVER_CHAT_STREAM_DISABLED",
      recoverable: true,
    });
    expect(called).toBe(false);
  });
});

function createMockClient(
  chatOverrides: Partial<ChatApi>,
  options: Partial<NeoChatApiClient> = {},
): NeoChatApiClient {
  const chat: ChatApi = {
    async createConversation() {
      throw new Error("createConversation not mocked");
    },
    async listConversations() {
      throw new Error("listConversations not mocked");
    },
    async appendUserMessage() {
      throw new Error("appendUserMessage not mocked");
    },
    async listMessages() {
      throw new Error("listMessages not mocked");
    },
    async streamAssistantMessage() {
      return { status: "unsupported" };
    },
    async cancelRun() {
      return { status: "unsupported" };
    },
    ...chatOverrides,
  };

  return {
    mode: options.mode ?? "server",
    config: options.config ?? resolvedConfig,
    capabilities: options.capabilities ?? capabilities,
    chat,
  };
}

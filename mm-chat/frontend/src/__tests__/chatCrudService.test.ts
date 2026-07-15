import { describe, expect, it } from "vitest";
import { createNeoChatApiClient } from "../services/api/client";
import {
  createChatCrudService,
  mapChatMessageDtoToMessage,
  mapConversationDtoToSession,
  modelRefToModelString,
  modelStringToModelRef,
  parseServerTimestamp,
} from "../services/api/chatCrudService";
import type {
  ApiCapabilities,
  ChatApi,
  ChatMessageDTO,
  ConversationDTO,
  FileApi,
  NeoChatApiClient,
  ResolvedApiClientConfig,
} from "../services/api/client";

const capabilities = {
  chatCrud: true,
  chatStream: false,
  files: false,
  auth: false,
  imports: false,
  rag: false,
  plugins: false,
  providerSettings: false,
  agents: false,
} satisfies ApiCapabilities;

const resolvedConfig = {
  mode: "server",
  requestedMode: "server",
  baseUrl: "http://backend.test",
  networkEdge: "direct-cors",
  serverConfigured: true,
  warnings: [],
} satisfies ResolvedApiClientConfig;

const conversationDto: ConversationDTO = {
  id: "c1",
  title: "Server Chat",
  status: "active",
  modelRef: { providerId: "openai", modelId: "gpt-5.5" },
  messageCount: 2,
  systemInstruction: "server instruction",
  pinned: true,
  config: {
    useSearch: true,
    useReasoning: "bad",
    activePlugins: ["writer", 42],
    internalTrace: "must-not-leak",
  },
  createdAt: "2026-07-08T00:00:00Z",
  updatedAt: "2026-07-08T00:01:00Z",
};

const userMessageDto: ChatMessageDTO = {
  id: "m1",
  conversationId: "c1",
  role: "user",
  status: "completed",
  content: "hello",
  sequenceNo: 1,
  attachments: [],
  outputBlocks: [],
  metadata: {},
  createdAt: "2026-07-08T00:00:01Z",
  updatedAt: "2026-07-08T00:00:01Z",
  completedAt: "2026-07-08T00:00:01Z",
};

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
      fileId: "file/1",
      fileName: "report.pdf",
      mimeType: "application/pdf",
      size: 123,
      sha256: "abc",
      purpose: "input",
      downloadUrl: "https://object-store.example/leak",
    },
  ],
  outputBlocks: [{ id: "block-1", type: "text", content: "hi" }],
  metadata: {},
  parentMessageId: "m1",
  createdAt: "2026-07-08T00:00:02Z",
  updatedAt: "2026-07-08T00:00:02Z",
  completedAt: "2026-07-08T00:00:02Z",
};

describe("chat CRUD DTO mappers", () => {
  it("maps conversation DTOs to legacy Session metadata", () => {
    const session = mapConversationDtoToSession(conversationDto);

    expect(session).toMatchObject({
      id: "c1",
      title: "Server Chat",
      messageCount: 2,
      model: "openai:gpt-5.5",
      pinned: true,
      systemInstruction: "server instruction",
      config: { useSearch: true, activePlugins: ["writer"] },
    });
    expect(session.config).not.toHaveProperty("internalTrace");
    expect(session.config).not.toHaveProperty("useReasoning");
    expect(session.updatedAt).toBe(Date.parse("2026-07-08T00:01:00Z"));
  });

  it("maps user and assistant messages to legacy Message roles", () => {
    expect(mapChatMessageDtoToMessage(userMessageDto)).toMatchObject({
      id: "m1",
      role: "user",
      content: "hello",
      timestamp: Date.parse("2026-07-08T00:00:01Z"),
    });

    const assistant = mapChatMessageDtoToMessage(assistantMessageDto, {
      baseUrl: "http://backend.test",
    });

    expect(assistant).toMatchObject({
      id: "m2",
      role: "model",
      content: "hi",
      model: "openai:gpt-5.5",
      attachments: [
        {
          id: "a1",
          source: "server",
          fileId: "file/1",
          fileName: "report.pdf",
          mimeType: "application/pdf",
          size: 123,
          sha256: "abc",
          purpose: "input",
          url: "http://backend.test/v1/files/file%2F1/content",
        },
      ],
      outputBlocks: [{ id: "block-1", type: "text", content: "hi" }],
      parentMessageId: "m1",
    });
    expect(JSON.stringify(assistant.attachments)).not.toContain(
      "object-store.example",
    );
  });

  it("fails closed on invalid timestamps and unsupported server roles", () => {
    expect(() => parseServerTimestamp("bad-date", "message.createdAt")).toThrow(
      /invalid message.createdAt/,
    );

    expect(() =>
      mapChatMessageDtoToMessage({ ...userMessageDto, role: "tool" }),
    ).toThrow(/cannot be rendered/);
  });

  it("converts model refs and legacy model strings", () => {
    expect(
      modelRefToModelString({ providerId: "openai", modelId: "gpt-5.5" }),
    ).toBe("openai:gpt-5.5");
    expect(modelStringToModelRef("openai:gpt-5.5")).toEqual({
      providerId: "openai",
      modelId: "gpt-5.5",
    });
    expect(modelStringToModelRef("SERVER_DEFAULT:gpt-5.5")).toEqual({
      providerId: "openai_compatible",
      modelId: "gpt-5.5",
    });
  });
});

describe("chat CRUD service gateway", () => {
  it("delegates server CRUD calls and maps results", async () => {
    const calls: string[] = [];
    const client = createMockClient({
      async createConversation(input) {
        calls.push(`create:${input.idempotencyKey}`);
        return conversationDto;
      },
      async listConversations() {
        calls.push("list-conversations");
        return [conversationDto];
      },
      async updateConversation(input) {
        calls.push(`update:${input.conversationId}:${input.title}`);
        return {
          ...conversationDto,
          title: input.title ?? conversationDto.title,
        };
      },
      async deleteConversation(conversationId) {
        calls.push(`delete:${conversationId}`);
      },
      async duplicateConversation(input) {
        calls.push(`duplicate:${input.conversationId}:${input.idempotencyKey}`);
        return {
          ...conversationDto,
          id: "c2",
          title: "Server Chat (Copy)",
          messageCount: 2,
        };
      },
      async generateConversationTitle(input) {
        calls.push(`title:${input.conversationId}:${input.modelRef?.modelId}`);
        return { title: "Generated Title" };
      },
      async updateMessage(input) {
        calls.push(
          `update-message:${input.conversationId}:${input.messageId}:${input.content}`,
        );
        return {
          ...assistantMessageDto,
          content: input.content,
          outputBlocks: [],
        };
      },
      async deleteMessage(input) {
        calls.push(
          `delete-message:${input.conversationId}:${input.messageId}:${input.scope}`,
        );
      },
      async appendUserMessage(input) {
        calls.push(`append:${input.conversationId}:${input.idempotencyKey}`);
        return userMessageDto;
      },
      async listMessages(conversationId) {
        calls.push(`list-messages:${conversationId}`);
        return [userMessageDto, assistantMessageDto];
      },
    });

    const service = createChatCrudService({ client });

    await expect(
      service.createConversation({ idempotencyKey: "conversation-key" }),
    ).resolves.toMatchObject({ id: "c1", model: "openai:gpt-5.5" });
    await expect(service.listConversations()).resolves.toHaveLength(1);
    await expect(
      service.updateConversation({
        conversationId: "c1",
        title: "Renamed",
      }),
    ).resolves.toMatchObject({ id: "c1", title: "Renamed" });
    await expect(service.deleteConversation("c1")).resolves.toBeUndefined();
    await expect(
      service.duplicateConversation({
        conversationId: "c1",
        idempotencyKey: "duplicate-key",
      }),
    ).resolves.toMatchObject({ id: "c2", title: "Server Chat (Copy)" });
    await expect(
      service.generateConversationTitle({
        conversationId: "c1",
        modelRef: { providerId: "openai", modelId: "gpt-title" },
      }),
    ).resolves.toBe("Generated Title");
    await expect(
      service.updateMessage({
        conversationId: "c1",
        messageId: "m2",
        content: "edited",
      }),
    ).resolves.toMatchObject({
      id: "m2",
      content: "edited",
      parentMessageId: "m1",
    });
    await expect(
      service.deleteMessage({
        conversationId: "c1",
        messageId: "m1",
        scope: "subsequent",
      }),
    ).resolves.toBeUndefined();
    await expect(
      service.appendUserMessage({
        conversationId: "c1",
        content: "hello",
        idempotencyKey: "message-key",
      }),
    ).resolves.toMatchObject({ id: "m1", role: "user" });
    await expect(service.listMessages("c1")).resolves.toEqual([
      expect.objectContaining({ id: "m1", role: "user" }),
      expect.objectContaining({ id: "m2", role: "model" }),
    ]);
    expect(calls).toEqual([
      "create:conversation-key",
      "list-conversations",
      "update:c1:Renamed",
      "delete:c1",
      "duplicate:c1:duplicate-key",
      "title:c1:gpt-title",
      "update-message:c1:m2:edited",
      "delete-message:c1:m1:subsequent",
      "append:c1:message-key",
      "list-messages:c1",
    ]);
  });

  it("fails closed when server CRUD is not enabled", async () => {
    let called = false;
    const chat = {
      async listConversations() {
        called = true;
        return [];
      },
    };

    for (const client of [
      createMockClient(chat, {
        mode: "local",
        capabilities: { ...capabilities, chatCrud: true },
      }),
      createMockClient(chat, {
        mode: "server",
        capabilities: { ...capabilities, chatCrud: false },
      }),
    ]) {
      const service = createChatCrudService({ client });

      await expect(service.listConversations()).rejects.toMatchObject({
        code: "SERVER_CHAT_CRUD_DISABLED",
        recoverable: true,
      });
    }

    expect(called).toBe(false);
  });
});

function createMockClient(
  chatOverrides: Partial<ChatApi>,
  options: Partial<NeoChatApiClient> = {},
): NeoChatApiClient {
  const defaultClient = createNeoChatApiClient();
  const chat: ChatApi = {
    async createConversation() {
      throw new Error("createConversation not mocked");
    },
    async listConversations() {
      throw new Error("listConversations not mocked");
    },
    async updateConversation() {
      throw new Error("updateConversation not mocked");
    },
    async deleteConversation() {
      throw new Error("deleteConversation not mocked");
    },
    async duplicateConversation() {
      throw new Error("duplicateConversation not mocked");
    },
    async generateConversationTitle() {
      throw new Error("generateConversationTitle not mocked");
    },
    async generateRelatedQuestions() {
      throw new Error("generateRelatedQuestions not mocked");
    },
    async updateMessage() {
      throw new Error("updateMessage not mocked");
    },
    async deleteMessage() {
      throw new Error("deleteMessage not mocked");
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
    async planTools() {
      return [];
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
    auth: options.auth ?? defaultClient.auth,
    settings: options.settings ?? defaultClient.settings,
    providers: options.providers ?? defaultClient.providers,
    byok: options.byok ?? defaultClient.byok,
    chat,
    files: options.files ?? createMockFileApi(),
    agents: options.agents ?? createMockAgentApi(),
  };
}

function createMockFileApi(): FileApi {
  return {
    async uploadFile() {
      throw new Error("uploadFile not mocked");
    },
    async getFile() {
      throw new Error("getFile not mocked");
    },
    async downloadFileContent() {
      throw new Error("downloadFileContent not mocked");
    },
    async deleteFile() {
      throw new Error("deleteFile not mocked");
    },
  };
}

function createMockAgentApi() {
  return {
    async listAgents() {
      throw new Error("listAgents not mocked");
    },
    async getAgentDetail() {
      throw new Error("getAgentDetail not mocked");
    },
  };
}

import { describe, expect, it } from "vitest";
import { createNeoChatApiClient } from "../services/api/client";
import {
  createFileService,
  mapFileRecordToServerAttachment,
  type UploadChatAttachmentInput,
  uploadMessageAttachmentsForServer,
} from "../services/api/fileService";
import type {
  ApiCapabilities,
  ChatApi,
  FileApi,
  FileRecordDTO,
  NeoChatApiClient,
  ResolvedApiClientConfig,
  UploadFileInput,
} from "../services/api/client";

const fileId = "00000000-0000-4000-8000-000000000001";
const fileRecord: FileRecordDTO = {
  id: fileId,
  fileName: "notes.txt",
  mimeType: "text/plain",
  size: 11,
  sha256: "64ec88ca00b268e5ba1a35678a1b5316d212f4f366b2477232534a8aeca37f3c",
  purpose: "chat",
  createdAt: "2026-07-08T00:00:00Z",
  downloadUrl: `/v1/files/${fileId}/content`,
};

const capabilities = {
  chatCrud: true,
  chatStream: true,
  files: true,
  auth: false,
  imports: false,
  rag: false,
  plugins: false,
  providerSettings: false,
  agents: false,
  voice: false,
  imageGeneration: false,
  codeExecution: false,
} satisfies ApiCapabilities;

const resolvedConfig = {
  mode: "server",
  requestedMode: "server",
  baseUrl: "http://backend.test",
  networkEdge: "direct-cors",
  serverConfigured: true,
  warnings: [],
} satisfies ResolvedApiClientConfig;

describe("Phase 11.4B file service gateway", () => {
  it("maps file records to server-backed legacy attachments", () => {
    expect(
      mapFileRecordToServerAttachment(fileRecord, {
        baseUrl: "http://backend.test",
      }),
    ).toEqual({
      id: fileId,
      source: "server",
      fileId,
      fileName: "notes.txt",
      mimeType: "text/plain",
      size: 11,
      sha256:
        "64ec88ca00b268e5ba1a35678a1b5316d212f4f366b2477232534a8aeca37f3c",
      purpose: "input",
      url: `http://backend.test/v1/files/${fileId}/content`,
    });
  });

  it("uploads chat attachments through the server file API", async () => {
    const calls: UploadFileInput[] = [];
    const service = createFileService({
      client: createMockClient({
        async uploadFile(input) {
          calls.push(input);
          return fileRecord;
        },
      }),
    });

    const blob = new Blob(["hello world"], { type: "text/plain" });

    await expect(
      service.uploadChatAttachment({
        file: blob,
        fileName: "notes.txt",
        conversationId: "conversation-1",
        clientFileId: "client-file-1",
        purpose: "chat",
      }),
    ).resolves.toMatchObject({
      source: "server",
      fileId,
      fileName: "notes.txt",
      purpose: "input",
    });

    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      file: blob,
      fileName: "notes.txt",
      purpose: "chat",
      conversationId: "conversation-1",
      clientFileId: "client-file-1",
    });
  });

  it("delegates metadata and binary downloads", async () => {
    const blob = new Blob(["hello world"], { type: "text/plain" });
    const service = createFileService({
      client: createMockClient({
        async getFile(id) {
          expect(id).toBe(fileId);
          return fileRecord;
        },
        async downloadFileContent(input) {
          expect(input).toEqual({ fileId, disposition: "attachment" });
          return { blob, contentType: "text/plain", size: 11 };
        },
      }),
    });

    await expect(service.getChatAttachment(fileId)).resolves.toMatchObject({
      source: "server",
      fileId,
    });
    await expect(
      service.downloadFileContent({ fileId, disposition: "attachment" }),
    ).resolves.toEqual({ blob, contentType: "text/plain", size: 11 });
  });

  it("fails closed when server files are disabled", async () => {
    for (const client of [
      createMockClient({}, { mode: "local" }),
      createMockClient({}, { capabilities: { ...capabilities, files: false } }),
    ]) {
      const service = createFileService({ client });

      await expect(
        service.uploadChatAttachment({
          file: new Blob(["hello"], { type: "text/plain" }),
          fileName: "notes.txt",
        }),
      ).rejects.toMatchObject({
        code: "SERVER_FILES_DISABLED",
        recoverable: true,
      });
    }
  });

  it("uploads inline message attachments and preserves server-backed ones", async () => {
    const calls: UploadChatAttachmentInput[] = [];
    const existing = mapFileRecordToServerAttachment(fileRecord);
    const uploaded = await uploadMessageAttachmentsForServer({
      conversationId: "conversation-1",
      attachments: [
        {
          id: "local-attachment",
          fileName: "hello.txt",
          mimeType: "text/plain",
          data: "aGVsbG8gd29ybGQ=",
        },
        existing,
      ],
      fileService: {
        async uploadChatAttachment(input) {
          calls.push(input);
          await expect(input.file.text()).resolves.toBe("hello world");
          return mapFileRecordToServerAttachment(fileRecord, {
            baseUrl: "http://backend.test",
            purpose: input.purpose,
          });
        },
      },
    });

    expect(uploaded).toEqual([
      expect.objectContaining({
        source: "server",
        fileId,
        fileName: "notes.txt",
        purpose: "input",
      }),
      existing,
    ]);
    expect(calls).toEqual([
      expect.objectContaining({
        fileName: "hello.txt",
        conversationId: "conversation-1",
        clientFileId: "local-attachment",
        purpose: "input",
      }),
    ]);
  });

  it("rejects URL-only attachments in server upload conversion", async () => {
    await expect(
      uploadMessageAttachmentsForServer({
        conversationId: "conversation-1",
        attachments: [
          {
            id: "remote-attachment",
            fileName: "remote.txt",
            mimeType: "text/plain",
            url: "https://example.test/file.txt",
          },
        ],
        fileService: {
          async uploadChatAttachment() {
            throw new Error("uploadChatAttachment should not be called");
          },
        },
      }),
    ).rejects.toMatchObject({
      code: "UNSUPPORTED_ATTACHMENT_SOURCE",
      recoverable: true,
    });
  });
});

function createMockClient(
  filesOverrides: Partial<FileApi>,
  options: Partial<NeoChatApiClient> = {},
): NeoChatApiClient {
  const defaultClient = createNeoChatApiClient();

  return {
    mode: options.mode ?? "server",
    config: options.config ?? resolvedConfig,
    capabilities: options.capabilities ?? capabilities,
    auth: options.auth ?? defaultClient.auth,
    settings: options.settings ?? defaultClient.settings,
    providers: options.providers ?? defaultClient.providers,
    byok: options.byok ?? defaultClient.byok,
    chat: createMockChatApi(),
    files: {
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
      ...filesOverrides,
    },
    plugins: options.plugins ?? defaultClient.plugins,
    agents: options.agents ?? createMockAgentApi(),
  };
}

function createMockChatApi(): ChatApi {
  return {
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

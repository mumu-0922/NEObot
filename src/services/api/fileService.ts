import {
  ApiClientError,
  createNeoChatApiClient,
  joinUrl,
  type ApiClientConfig,
  type DownloadedFileContent,
  type DownloadFileContentInput,
  type FileRecordDTO,
  type NeoChatApiClient,
} from "./client";
import {
  normalizeServerMessageAttachmentPurpose,
  type ServerBackedAttachment,
  type ServerMessageAttachmentPurpose,
} from "../../lib/utils/serverAttachments";

export interface UploadChatAttachmentInput {
  file: Blob;
  fileName?: string;
  conversationId?: string;
  workspaceId?: string;
  knowledgeCollectionId?: string;
  clientFileId?: string;
  purpose?: ServerMessageAttachmentPurpose | "chat" | "knowledge";
  signal?: AbortSignal;
}

export interface FileServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export interface FileService {
  mode: NeoChatApiClient["mode"];
  serverEnabled: boolean;
  uploadChatAttachment(
    input: UploadChatAttachmentInput,
  ): Promise<ServerBackedAttachment>;
  getChatAttachment(
    fileId: string,
    options?: { signal?: AbortSignal },
  ): Promise<ServerBackedAttachment>;
  downloadFileContent(
    input: DownloadFileContentInput,
  ): Promise<DownloadedFileContent>;
}

export function createFileService(
  options: FileServiceOptions = {},
): FileService {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const baseUrl = client.config.baseUrl;
  const serverEnabled =
    client.mode === "server" && client.capabilities.files === true;

  function requireServerFiles(): void {
    if (!serverEnabled) {
      throw new ApiClientError(
        "SERVER_FILES_DISABLED",
        "Server file API is not enabled for the current API mode.",
        { recoverable: true },
      );
    }
  }

  return {
    mode: client.mode,
    serverEnabled,

    async uploadChatAttachment(input) {
      requireServerFiles();
      const record = await client.files.uploadFile({
        file: input.file as Blob,
        fileName: input.fileName,
        purpose: "chat",
        conversationId: input.conversationId,
        workspaceId: input.workspaceId,
        knowledgeCollectionId: input.knowledgeCollectionId,
        clientFileId: input.clientFileId,
        signal: input.signal,
      });
      return mapFileRecordToServerAttachment(record, {
        baseUrl,
        purpose: input.purpose,
      });
    },

    async getChatAttachment(fileId, options = {}) {
      requireServerFiles();
      return mapFileRecordToServerAttachment(
        await client.files.getFile(fileId, options),
        { baseUrl },
      );
    },

    async downloadFileContent(input) {
      requireServerFiles();
      return client.files.downloadFileContent(input);
    },
  };
}

export function mapFileRecordToServerAttachment(
  record: FileRecordDTO,
  options: {
    baseUrl?: string;
    purpose?: ServerMessageAttachmentPurpose | "chat" | "knowledge";
  } = {},
): ServerBackedAttachment {
  return {
    id: record.id,
    source: "server",
    fileId: record.id,
    fileName: record.fileName || "download",
    mimeType: record.mimeType || "application/octet-stream",
    size: record.size,
    sha256: record.sha256,
    purpose: normalizeServerMessageAttachmentPurpose(options.purpose),
    url: joinUrl(
      options.baseUrl ?? "",
      `/v1/files/${encodeURIComponent(record.id)}/content`,
    ),
  };
}

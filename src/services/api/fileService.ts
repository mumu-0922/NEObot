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
import type { Attachment } from "../../types";
import {
  isServerBackedAttachment,
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

export interface UploadMessageAttachmentsForServerInput {
  attachments: Attachment[];
  conversationId: string;
  fileService?: Pick<FileService, "uploadChatAttachment">;
  signal?: AbortSignal;
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

export async function uploadMessageAttachmentsForServer({
  attachments,
  conversationId,
  fileService = createFileService(),
  signal,
}: UploadMessageAttachmentsForServerInput): Promise<ServerBackedAttachment[]> {
  const uploaded: ServerBackedAttachment[] = [];

  for (const attachment of attachments) {
    if (isServerBackedAttachment(attachment)) {
      uploaded.push(attachment);
      continue;
    }

    uploaded.push(
      await fileService.uploadChatAttachment({
        file: attachmentToBlob(attachment),
        fileName: attachment.fileName,
        conversationId,
        clientFileId: attachment.id,
        purpose: inferServerMessagePurpose(attachment),
        signal,
      }),
    );
  }

  return uploaded;
}

function attachmentToBlob(attachment: Attachment): Blob {
  if (!attachment.data) {
    throw new ApiClientError(
      "UNSUPPORTED_ATTACHMENT_SOURCE",
      "Server mode can only upload inline/base64 attachments or reuse server-backed file attachments.",
      { recoverable: true },
    );
  }

  const bytes = decodeBase64Bytes(attachment.data);
  const buffer = new ArrayBuffer(bytes.byteLength);
  new Uint8Array(buffer).set(bytes);
  return new Blob([buffer], {
    type: attachment.mimeType || "application/octet-stream",
  });
}

function decodeBase64Bytes(data: string): Uint8Array {
  const normalized = data.includes(",") ? data.split(",").pop() || "" : data;
  const binary = atob(normalized);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function inferServerMessagePurpose(
  attachment: Attachment,
): ServerMessageAttachmentPurpose {
  if (attachment.mimeType.startsWith("image/")) return "image";
  return "input";
}

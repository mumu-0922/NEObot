import { ApiClientError } from "../errors";
import type {
  DownloadedFileContent,
  DownloadFileContentInput,
  FileApi,
  FilePurpose,
  FileRecordDTO,
  UploadFileInput,
} from "../types";
import type { HttpClient } from "./httpClient";

const filesPath = "/v1/files";

export function createServerFileApiShell(httpClient: HttpClient): FileApi {
  return {
    async uploadFile(input: UploadFileInput): Promise<FileRecordDTO> {
      return normalizeFileRecord(
        await httpClient.requestMultipartJson<unknown>(filesPath, {
          method: "POST",
          formData: createUploadFormData(input),
          signal: input.signal,
        }),
      );
    },

    async getFile(
      fileId: string,
      options: { signal?: AbortSignal } = {},
    ): Promise<FileRecordDTO> {
      return normalizeFileRecord(
        await httpClient.requestJson<unknown>(filePath(fileId), {
          signal: options.signal,
        }),
      );
    },

    async downloadFileContent(
      input: DownloadFileContentInput,
    ): Promise<DownloadedFileContent> {
      return httpClient.requestBinary(downloadContentPath(input), {
        signal: input.signal,
      });
    },

    async deleteFile(
      fileId: string,
      options: { signal?: AbortSignal } = {},
    ): Promise<void> {
      await httpClient.requestJson<void>(filePath(fileId), {
        method: "DELETE",
        signal: options.signal,
      });
    },
  };
}

function createUploadFormData(input: UploadFileInput): FormData {
  if (!input.file) {
    throw new ApiClientError("FILE_REQUIRED", "file is required");
  }
  const purpose = normalizePurpose(input.purpose);
  const formData = new FormData();
  formData.append("file", input.file, uploadFileName(input));
  formData.append("purpose", purpose);
  appendOptionalField(formData, "conversationId", input.conversationId);
  appendOptionalField(formData, "workspaceId", input.workspaceId);
  appendOptionalField(
    formData,
    "knowledgeCollectionId",
    input.knowledgeCollectionId,
  );
  appendOptionalField(formData, "clientFileId", input.clientFileId);
  return formData;
}

function normalizePurpose(purpose: FilePurpose): FilePurpose {
  const normalized = String(purpose ?? "").trim() as FilePurpose;
  if (!normalized) {
    throw new ApiClientError(
      "INVALID_FILE_PURPOSE",
      "file purpose is required",
    );
  }
  return normalized;
}

function uploadFileName(input: UploadFileInput): string {
  const explicitName = input.fileName?.trim();
  if (explicitName) return explicitName;
  const blobName = (input.file as Blob & { name?: string }).name?.trim();
  return blobName || "upload.bin";
}

function appendOptionalField(
  formData: FormData,
  name: string,
  value: string | undefined,
): void {
  const normalized = value?.trim();
  if (normalized) formData.append(name, normalized);
}

function filePath(fileId: string): string {
  const normalized = fileId.trim();
  if (!normalized) {
    throw new ApiClientError("INVALID_FILE_ID", "file id is required");
  }
  return `${filesPath}/${encodeURIComponent(normalized)}`;
}

function downloadContentPath(input: DownloadFileContentInput): string {
  const query =
    input.disposition === "attachment" ? "?disposition=attachment" : "";
  return `${filePath(input.fileId)}/content${query}`;
}

function normalizeFileRecord(value: unknown): FileRecordDTO {
  if (!value || typeof value !== "object") {
    throw invalidFileResponse();
  }
  const record = value as Record<string, unknown>;
  const id = readString(record, "id");
  const fileName = readString(record, "fileName");
  const mimeType = readString(record, "mimeType");
  const size = readFiniteNumber(record, "size");
  const sha256 = readString(record, "sha256");
  const purpose = readString(record, "purpose") as FilePurpose;
  const createdAt = readString(record, "createdAt");
  const downloadUrl = readString(record, "downloadUrl");

  if (!isFilePurpose(purpose)) {
    throw invalidFileResponse();
  }
  if (!isSafeDownloadUrl(downloadUrl, id)) {
    throw new ApiClientError(
      "INVALID_SERVER_RESPONSE",
      "Server returned an unsafe file download URL.",
      { recoverable: true },
    );
  }

  return {
    id,
    fileName,
    mimeType,
    size,
    sha256,
    purpose,
    createdAt,
    downloadUrl,
  };
}

function readString(record: Record<string, unknown>, field: string): string {
  const value = record[field];
  if (typeof value !== "string") throw invalidFileResponse();
  return value;
}

function readFiniteNumber(
  record: Record<string, unknown>,
  field: string,
): number {
  const value = record[field];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw invalidFileResponse();
  }
  return value;
}

function isSafeDownloadUrl(downloadUrl: string, id: string): boolean {
  if (!isUuid(id)) return false;
  return downloadUrl === `${filesPath}/${encodeURIComponent(id)}/content`;
}

function isUuid(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
    value,
  );
}

function isFilePurpose(value: string): value is FilePurpose {
  return (
    value === "chat" ||
    value === "workspace" ||
    value === "knowledge" ||
    value === "image" ||
    value === "audio" ||
    value === "export"
  );
}

function invalidFileResponse(): ApiClientError {
  return new ApiClientError(
    "INVALID_SERVER_RESPONSE",
    "Server returned invalid file metadata.",
    { recoverable: true },
  );
}

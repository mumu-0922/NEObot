import type {
  BrowserImportApi,
  BrowserImportBatchStatus,
  BrowserImportCommitResponse,
  BrowserImportPackageInput,
  BrowserImportPreviewResponse,
} from "../types";
import type { HttpClient } from "./httpClient";

const importPath = "/v1/import/browser";
const importPackageFileName = "neo-chat-browser-import-v2.zip";

export function createServerImportApiShell(
  httpClient: HttpClient,
): BrowserImportApi {
  return {
    async previewBrowserImport(
      input: BrowserImportPackageInput,
    ): Promise<BrowserImportPreviewResponse> {
      return httpClient.requestMultipartJson<BrowserImportPreviewResponse>(
        `${importPath}/preview`,
        {
          method: "POST",
          formData: createImportFormData(input),
          signal: input.signal,
        },
      );
    },

    async commitBrowserImport(
      input: BrowserImportPackageInput,
    ): Promise<BrowserImportCommitResponse> {
      return httpClient.requestMultipartJson<BrowserImportCommitResponse>(
        importPath,
        {
          method: "POST",
          formData: createImportFormData(input),
          signal: input.signal,
        },
      );
    },

    async getBrowserImportBatch(
      batchId: string,
      options: { signal?: AbortSignal } = {},
    ): Promise<BrowserImportBatchStatus> {
      return httpClient.requestJson<BrowserImportBatchStatus>(
        batchPath(batchId),
        { signal: options.signal },
      );
    },

    async rollbackBrowserImportBatch(
      batchId: string,
      options: { signal?: AbortSignal } = {},
    ): Promise<void> {
      await httpClient.requestJson<void>(batchPath(batchId), {
        method: "DELETE",
        signal: options.signal,
      });
    },
  };
}

function createImportFormData(input: BrowserImportPackageInput): FormData {
  const formData = new FormData();
  formData.append(
    "package",
    input.package,
    input.fileName?.trim() || importPackageFileName,
  );
  return formData;
}

function batchPath(batchId: string): string {
  return `${importPath}/${encodeURIComponent(batchId.trim())}`;
}

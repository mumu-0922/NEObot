import {
  ApiClientError,
  createNeoChatApiClient,
  type ApiClientConfig,
  type BrowserImportBatchStatus,
  type BrowserImportCommitResponse,
  type BrowserImportPreviewResponse,
  type NeoChatApiClient,
} from "./client";

export interface BrowserImportServiceOptions {
  config?: ApiClientConfig;
  client?: NeoChatApiClient;
}

export interface BrowserImportService {
  mode: NeoChatApiClient["mode"];
  serverEnabled: boolean;
  preview(
    pkg: Blob,
    options?: { fileName?: string; signal?: AbortSignal },
  ): Promise<BrowserImportPreviewResponse>;
  commit(
    pkg: Blob,
    options?: { fileName?: string; signal?: AbortSignal },
  ): Promise<BrowserImportCommitResponse>;
  getBatch(
    batchId: string,
    options?: { signal?: AbortSignal },
  ): Promise<BrowserImportBatchStatus>;
  rollbackBatch(
    batchId: string,
    options?: { signal?: AbortSignal },
  ): Promise<void>;
}

export function createBrowserImportService(
  options: BrowserImportServiceOptions = {},
): BrowserImportService {
  const client = options.client ?? createNeoChatApiClient(options.config);
  const serverEnabled =
    client.mode === "server" &&
    client.capabilities.imports === true &&
    Boolean(client.imports);

  function requireServerImports(): void {
    if (!serverEnabled) {
      throw new ApiClientError(
        "SERVER_IMPORTS_DISABLED",
        "Browser import is only available when server API mode is configured.",
        { recoverable: true },
      );
    }
  }

  return {
    mode: client.mode,
    serverEnabled,

    async preview(pkg, request = {}) {
      requireServerImports();
      return client.imports!.previewBrowserImport({
        package: pkg,
        fileName: request.fileName,
        signal: request.signal,
      });
    },

    async commit(pkg, request = {}) {
      requireServerImports();
      return client.imports!.commitBrowserImport({
        package: pkg,
        fileName: request.fileName,
        signal: request.signal,
      });
    },

    async getBatch(batchId, request = {}) {
      requireServerImports();
      return client.imports!.getBrowserImportBatch(batchId, request);
    },

    async rollbackBatch(batchId, request = {}) {
      requireServerImports();
      await client.imports!.rollbackBrowserImportBatch(batchId, request);
    },
  };
}

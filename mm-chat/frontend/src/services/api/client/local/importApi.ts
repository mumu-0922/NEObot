import { unsupportedFeature } from "../errors";
import type {
  BrowserImportApi,
  BrowserImportBatchStatus,
  BrowserImportCommitResponse,
  BrowserImportPreviewResponse,
} from "../types";

export function createLocalImportApiShell(): BrowserImportApi {
  return {
    async previewBrowserImport(): Promise<BrowserImportPreviewResponse> {
      throw unsupportedFeature("local browser import preview");
    },
    async commitBrowserImport(): Promise<BrowserImportCommitResponse> {
      throw unsupportedFeature("local browser import commit");
    },
    async getBrowserImportBatch(): Promise<BrowserImportBatchStatus> {
      throw unsupportedFeature("local browser import batch status");
    },
    async rollbackBrowserImportBatch(): Promise<void> {
      throw unsupportedFeature("local browser import rollback");
    },
  };
}

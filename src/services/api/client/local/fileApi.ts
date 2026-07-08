import { unsupportedFeature } from "../errors";
import type { DownloadedFileContent, FileApi, FileRecordDTO } from "../types";

export function createLocalFileApiShell(): FileApi {
  return {
    async uploadFile(): Promise<FileRecordDTO> {
      throw unsupportedFeature("local file adapter wiring");
    },
    async getFile(): Promise<FileRecordDTO> {
      throw unsupportedFeature("local file adapter wiring");
    },
    async downloadFileContent(): Promise<DownloadedFileContent> {
      throw unsupportedFeature("local file adapter wiring");
    },
    async deleteFile(): Promise<void> {
      throw unsupportedFeature("local file adapter wiring");
    },
  };
}

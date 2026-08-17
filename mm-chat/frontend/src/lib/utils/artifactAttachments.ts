import type { Attachment } from "@/types";
import {
  createFileService,
  type FileService,
} from "@/services/api/fileService";
import { triggerBlobDownload } from "@/lib/utils/blobDownload";
import { isServerArtifactAttachment } from "@/lib/utils/serverAttachments";

export interface DownloadServerArtifactOptions {
  fileService?: Pick<FileService, "downloadFileContent">;
  saveBlob?: typeof triggerBlobDownload;
  signal?: AbortSignal;
}

export async function downloadServerArtifact(
  attachment: Attachment,
  options: DownloadServerArtifactOptions = {},
): Promise<void> {
  if (!isServerArtifactAttachment(attachment)) {
    throw new Error("Attachment is not a server artifact.");
  }
  const fileService = options.fileService ?? createFileService();
  const downloaded = await fileService.downloadFileContent({
    fileId: attachment.fileId,
    disposition: "attachment",
    signal: options.signal,
  });
  (options.saveBlob ?? triggerBlobDownload)(
    downloaded.blob,
    attachment.fileName,
  );
}

export function formatArtifactSize(size: number | undefined): string {
  if (typeof size !== "number" || !Number.isFinite(size) || size < 0) {
    return "";
  }
  if (size < 1024) return `${Math.floor(size)} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

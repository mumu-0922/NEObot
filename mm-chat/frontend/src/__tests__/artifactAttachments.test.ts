import { describe, expect, it, vi } from "vitest";
import {
  downloadServerArtifact,
  formatArtifactSize,
} from "../lib/utils/artifactAttachments";
import type { Attachment } from "../types";

const artifact = {
  id: "attachment-1",
  source: "server",
  fileId: "55555555-5555-4555-8555-555555555555",
  fileName: "result.csv",
  mimeType: "text/csv",
  size: 11,
  sha256: "abc",
  purpose: "output",
  url: "http://backend.test/v1/files/55555555-5555-4555-8555-555555555555/content",
} satisfies Attachment;

describe("server artifact downloads", () => {
  it("downloads through the authenticated File API and saves the returned Blob", async () => {
    const blob = new Blob(["hello world"], { type: "text/csv" });
    const downloadFileContent = vi.fn(async () => ({
      blob,
      contentType: "text/csv",
      size: 11,
    }));
    const saveBlob = vi.fn();

    await downloadServerArtifact(artifact, {
      fileService: { downloadFileContent },
      saveBlob,
    });

    expect(downloadFileContent).toHaveBeenCalledWith({
      fileId: artifact.fileId,
      disposition: "attachment",
      signal: undefined,
    });
    expect(saveBlob).toHaveBeenCalledWith(blob, "result.csv");
  });

  it("propagates missing/deleted file failures and never reports a saved file", async () => {
    const failure = new Error("FILE_NOT_FOUND");
    const saveBlob = vi.fn();

    await expect(
      downloadServerArtifact(artifact, {
        fileService: {
          downloadFileContent: vi.fn(async () => {
            throw failure;
          }),
        },
        saveBlob,
      }),
    ).rejects.toBe(failure);
    expect(saveBlob).not.toHaveBeenCalled();
  });

  it("rejects ordinary input attachments and formats bounded size labels", async () => {
    await expect(
      downloadServerArtifact(
        { ...artifact, purpose: "input" },
        {
          fileService: {
            downloadFileContent: vi.fn(),
          },
        },
      ),
    ).rejects.toThrow("not a server artifact");
    expect(formatArtifactSize(999)).toBe("999 B");
    expect(formatArtifactSize(1536)).toBe("1.5 KB");
    expect(formatArtifactSize(2 * 1024 * 1024)).toBe("2.0 MB");
  });
});

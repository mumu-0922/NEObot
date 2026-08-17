import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import en from "../i18n/locales/en";
import zh from "../i18n/locales/zh";

describe("MessageAttachmentView composition", () => {
  it("opens image attachments through the shared zoom preview", () => {
    const attachmentView = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageAttachmentView.tsx"),
      "utf8",
    );
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );

    expect(attachmentView).toContain('type="button"');
    expect(attachmentView).toContain("cursor-zoom-in");
    expect(attachmentView).toContain("onClick={onImageClick}");
    expect(messageItem).toContain("openImagePreview(previewImages, newIndex)");
  });

  it("exposes readable document cards with localized open actions", () => {
    const attachmentView = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageAttachmentView.tsx"),
      "utf8",
    );
    const messageItem = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageItem.tsx"),
      "utf8",
    );
    expect(attachmentView).toContain("markdown-file-card");
    expect(attachmentView).toContain("isTextDocumentMimeType");
    expect(attachmentView).toContain("onDocumentClick");
    expect(attachmentView).toContain("openDocumentAttachmentAria");
    expect(attachmentView).not.toContain("markdown-file-type-badge");
    expect(attachmentView).not.toContain("isParsedMarkdown");
    expect(messageItem).toContain("decodeAttachmentText");
    expect(messageItem).toContain('readingMode === "attachment"');
    expect(messageItem).toContain("copyFileAria");
    expect(messageItem).toContain("markdown-file-type-badge");
    expect(en.Message.openDocumentAttachment).toBe("Open document");
    expect(en.Message.readingAttachment).toContain("{name}");
    expect(zh.Message.openDocumentAttachment).toBe("打开文档");
    expect(zh.Message.readingAttachment).toContain("{name}");
  });

  it("renders output artifacts as authenticated download cards", () => {
    const attachmentView = readFileSync(
      resolve(process.cwd(), "src/components/chat/MessageAttachmentView.tsx"),
      "utf8",
    );
    const artifactHelper = readFileSync(
      resolve(process.cwd(), "src/lib/utils/artifactAttachments.ts"),
      "utf8",
    );

    expect(attachmentView).toContain("isServerArtifactAttachment");
    expect(attachmentView).toContain("downloadServerArtifact");
    expect(attachmentView).toContain('aria-live="polite"');
    expect(attachmentView).toContain("artifactDownloadFailed");
    expect(artifactHelper).toContain("downloadFileContent");
    expect(artifactHelper).toContain('disposition: "attachment"');
    expect(artifactHelper).toContain("triggerBlobDownload");
    expect(en.Message.artifactDownloadFailed).toContain("deleted");
    expect(zh.Message.artifactDownloadFailed).toContain("删除");
  });
});

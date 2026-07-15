import { describe, expect, it } from "vitest";
import type { Attachment } from "../types";
import {
  normalizeServerMessageAttachmentPurpose,
  toServerMessageAttachments,
  type ServerBackedAttachment,
} from "../lib/utils/serverAttachments";

describe("Phase 11.4B server attachment mapper", () => {
  it("converts server-backed attachments to Go message attachment refs", () => {
    const attachment: ServerBackedAttachment = {
      id: "file-attachment",
      source: "server",
      fileId: "00000000-0000-4000-8000-000000000001",
      fileName: "notes.txt",
      mimeType: "text/plain",
      size: 11,
      sha256: "sha",
      purpose: "chat",
      url: "http://backend.test/v1/files/00000000-0000-4000-8000-000000000001/content",
    };

    expect(toServerMessageAttachments([attachment])).toEqual([
      {
        source: "server",
        fileId: "00000000-0000-4000-8000-000000000001",
        purpose: "input",
      },
    ]);
  });

  it("returns undefined for empty attachment lists", () => {
    expect(toServerMessageAttachments([])).toBeUndefined();
  });

  it("normalizes known message attachment purpose aliases", () => {
    expect(normalizeServerMessageAttachmentPurpose(undefined)).toBe("input");
    expect(normalizeServerMessageAttachmentPurpose("chat")).toBe("input");
    expect(normalizeServerMessageAttachmentPurpose("knowledge")).toBe(
      "knowledge_source",
    );
    expect(normalizeServerMessageAttachmentPurpose("image")).toBe("image");
  });

  it("fails closed for non-server or unsupported attachments", () => {
    const localAttachment: Attachment = {
      id: "local",
      fileName: "local.txt",
      mimeType: "text/plain",
      data: "aGVsbG8=",
    };
    const invalidPurpose: ServerBackedAttachment = {
      id: "server",
      source: "server",
      fileId: "00000000-0000-4000-8000-000000000001",
      fileName: "server.txt",
      mimeType: "text/plain",
      purpose: "export",
    };

    expect(() => toServerMessageAttachments([localAttachment])).toThrow(
      /server-backed/,
    );
    expect(() => toServerMessageAttachments([invalidPurpose])).toThrow(
      /purpose/,
    );
  });
});

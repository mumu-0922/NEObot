import { describe, expect, it } from "vitest";
import {
  decodeAttachmentText,
  decodeBase64Text,
  isTextDocumentMimeType,
} from "../lib/utils/documentAttachments";

describe("chat document attachments", () => {
  it("recognizes text and structured text MIME types", () => {
    expect(isTextDocumentMimeType("text/markdown")).toBe(true);
    expect(isTextDocumentMimeType("application/json")).toBe(true);
    expect(isTextDocumentMimeType("application/problem+json")).toBe(true);
    expect(isTextDocumentMimeType("application/pdf")).toBe(false);
    expect(isTextDocumentMimeType(undefined)).toBe(false);
  });

  it("decodes UTF-8 attachment text", () => {
    const encoded = btoa(
      String.fromCharCode(...new TextEncoder().encode("hello\n知识库")),
    );

    expect(decodeBase64Text(encoded)).toBe("hello\n知识库");
    expect(decodeAttachmentText({ data: encoded })).toBe("hello\n知识库");
    expect(decodeAttachmentText({})).toBe("");
  });
});

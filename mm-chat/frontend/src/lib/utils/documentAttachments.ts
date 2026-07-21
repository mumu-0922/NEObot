import type { Attachment } from "../../types";

const TEXT_MIME_TYPES = new Set([
  "application/javascript",
  "application/json",
  "application/ld+json",
  "application/sql",
  "application/typescript",
  "application/xhtml+xml",
  "application/xml",
  "application/x-httpd-php",
  "application/x-sh",
  "application/x-yaml",
  "text/markdown",
]);

function normalizeMimeType(value: string | undefined, fallback = "text/plain") {
  return (value || fallback).trim().toLowerCase();
}

export function isTextDocumentMimeType(mimeType: string | undefined): boolean {
  const normalized = normalizeMimeType(mimeType, "");
  if (!normalized) return false;
  return (
    normalized.startsWith("text/") ||
    TEXT_MIME_TYPES.has(normalized) ||
    normalized.endsWith("+json") ||
    normalized.endsWith("+xml")
  );
}

export function decodeBase64Text(data: string): string {
  const binary = atob(data);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return new TextDecoder().decode(bytes);
}

export function decodeAttachmentText(attachment: Pick<Attachment, "data">) {
  return attachment.data ? decodeBase64Text(attachment.data) : "";
}

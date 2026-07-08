import type { Attachment } from "../../types";
import {
  ApiClientError,
  type AppendUserMessageInput,
} from "../../services/api/client";

export type ServerMessageAttachmentPurpose =
  "input" | "image" | "knowledge_source";

export interface ServerBackedAttachment extends Attachment {
  source: "server";
  fileId: string;
  size?: number;
  sha256?: string;
  purpose?: ServerMessageAttachmentPurpose | string;
}

export function isServerBackedAttachment(
  attachment: Attachment,
): attachment is ServerBackedAttachment {
  const candidate = attachment as Partial<ServerBackedAttachment>;
  return (
    candidate.source === "server" &&
    typeof candidate.fileId === "string" &&
    candidate.fileId.trim() !== ""
  );
}

export function toServerMessageAttachments(
  attachments: Attachment[],
): AppendUserMessageInput["attachments"] {
  if (attachments.length === 0) return undefined;

  return attachments.map((attachment) => {
    if (!isServerBackedAttachment(attachment)) {
      throw new ApiClientError(
        "UNSUPPORTED_ATTACHMENT_SOURCE",
        "Server mode can only send server-backed file attachments.",
      );
    }

    return {
      source: "server" as const,
      fileId: attachment.fileId.trim(),
      purpose: normalizeServerMessageAttachmentPurpose(attachment.purpose),
    };
  });
}

export function normalizeServerMessageAttachmentPurpose(
  purpose: string | undefined,
): ServerMessageAttachmentPurpose {
  const normalized = purpose?.trim();
  if (!normalized || normalized === "chat") return "input";
  if (normalized === "knowledge") return "knowledge_source";
  if (
    normalized === "input" ||
    normalized === "image" ||
    normalized === "knowledge_source"
  ) {
    return normalized;
  }

  throw new ApiClientError(
    "INVALID_ATTACHMENT_PURPOSE",
    "Server attachment purpose is not supported.",
  );
}

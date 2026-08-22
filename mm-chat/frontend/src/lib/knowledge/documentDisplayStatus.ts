import type {
  KnowledgeDocumentDTO,
  KnowledgeDocumentStatus,
} from "@/services/api/client";

export type KnowledgeDocumentDisplayStatus = KnowledgeDocumentStatus | "failed";

export const getKnowledgeDocumentDisplayStatus = (
  document: KnowledgeDocumentDTO,
): KnowledgeDocumentDisplayStatus => {
  if (
    document.currentVersion === undefined &&
    document.pendingVersion?.status === "failed"
  ) {
    return "failed";
  }
  return document.status;
};

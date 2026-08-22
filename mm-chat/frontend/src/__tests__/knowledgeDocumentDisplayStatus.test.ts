import { describe, expect, it } from "vitest";
import { getKnowledgeDocumentDisplayStatus } from "@/lib/knowledge/documentDisplayStatus";
import type {
  KnowledgeDocumentDTO,
  KnowledgeDocumentVersionDTO,
} from "@/services/api/client";

const version = (
  status: KnowledgeDocumentVersionDTO["status"],
): KnowledgeDocumentVersionDTO => ({
  id: `version-${status}`,
  sourceVersion: 1,
  file: {
    id: "file-1",
    name: "source.docx",
    mimeType:
      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    byteSize: 1024,
  },
  status,
  createdAt: "2026-08-22T00:00:00Z",
  updatedAt: "2026-08-22T00:00:00Z",
});

const document = (
  overrides: Partial<KnowledgeDocumentDTO>,
): KnowledgeDocumentDTO => ({
  id: "document-1",
  collectionId: "collection-1",
  status: "processing",
  createdAt: "2026-08-22T00:00:00Z",
  updatedAt: "2026-08-22T00:00:00Z",
  ...overrides,
});

describe("knowledge document display status", () => {
  it("surfaces a failed first version instead of indefinite processing", () => {
    expect(
      getKnowledgeDocumentDisplayStatus(
        document({ pendingVersion: version("failed") }),
      ),
    ).toBe("failed");
  });

  it("keeps an active current version available after replacement failure", () => {
    expect(
      getKnowledgeDocumentDisplayStatus(
        document({
          status: "active",
          currentVersion: version("active"),
          pendingVersion: version("failed"),
        }),
      ),
    ).toBe("active");
  });

  it("preserves ordinary processing state for nonterminal work", () => {
    expect(
      getKnowledgeDocumentDisplayStatus(
        document({ pendingVersion: version("uploaded") }),
      ),
    ).toBe("processing");
  });
});

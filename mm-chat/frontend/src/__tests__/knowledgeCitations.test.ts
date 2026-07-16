import { describe, expect, it } from "vitest";
import { normalizeMessageKnowledgeMetadata } from "@/lib/knowledge/citations";
import {
  createKnowledgeCollectionAttachment,
  createKnowledgeFileAttachment,
  getKnowledgeAttachmentCollectionIds,
} from "@/lib/utils/knowledgeAttachments";

describe("Knowledge citation metadata", () => {
  it("normalizes strict citation metadata from server messages", () => {
    const knowledge = normalizeMessageKnowledgeMetadata({
      knowledge: {
        mode: "strict",
        outcome: "answered",
        selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
        citationCount: 1,
        citations: [
          {
            id: "cit_1",
            marker: "[1]",
            documentId: "doc_1",
            locator: { page: 1 },
            snippet: "alpha evidence source",
            rankScore: 0.9,
          },
        ],
      },
    });

    expect(knowledge).toMatchObject({
      mode: "strict",
      outcome: "answered",
      citationCount: 1,
      evidenceUsed: true,
      selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
      citations: [
        {
          id: "cit_1",
          marker: "[1]",
          snippet: "alpha evidence source",
          locator: { page: 1 },
        },
      ],
    });
  });

  it("preserves optional degradation metadata without citations", () => {
    const knowledge = normalizeMessageKnowledgeMetadata({
      knowledge: {
        mode: "optional",
        outcome: "degraded",
        selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
        citationCount: 0,
        evidenceUsed: false,
        degradationReason: "no_verified_knowledge_evidence",
      },
    });

    expect(knowledge).toMatchObject({
      mode: "optional",
      outcome: "degraded",
      citationCount: 0,
      evidenceUsed: false,
      degradationReason: "no_verified_knowledge_evidence",
      citations: [],
    });
  });

  it("extracts selected collection ids from collection and file attachments", () => {
    const ids = getKnowledgeAttachmentCollectionIds([
      createKnowledgeCollectionAttachment({
        collectionId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        collectionName: "Alpha",
      }),
      createKnowledgeFileAttachment({
        collectionId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        fileId: "file-1",
        fileName: "alpha.pdf",
      }),
      createKnowledgeFileAttachment({
        collectionId: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
        fileId: "file-2",
        fileName: "beta.pdf",
      }),
    ]);

    expect(ids).toEqual([
      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    ]);
  });
});

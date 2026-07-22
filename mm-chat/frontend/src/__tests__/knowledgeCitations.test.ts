import { describe, expect, it } from "vitest";
import {
  normalizeMessageKnowledgeMetadata,
  reconcileMessageKnowledgeContent,
} from "@/lib/knowledge/citations";
import {
  createKnowledgeCollectionAttachment,
  createKnowledgeFileAttachment,
  getKnowledgeAttachmentCollectionIds,
} from "@/lib/utils/knowledgeAttachments";

describe("Knowledge citation metadata", () => {
  it("normalizes Auto citation metadata from server messages", () => {
    const knowledge = normalizeMessageKnowledgeMetadata({
      knowledge: {
        mode: "auto",
        outcome: "answered",
        selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
        citationCount: 1,
        citations: [
          {
            id: "cit_1",
            marker: "[K1]",
            documentId: "doc_1",
            locator: { page: 1 },
            snippet: "alpha evidence source",
            rankScore: 0.9,
          },
        ],
      },
    });

    expect(knowledge).toMatchObject({
      mode: "auto",
      outcome: "answered",
      citationCount: 1,
      evidenceUsed: true,
      selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
      citations: [
        {
          id: "cit_1",
          marker: "[K1]",
          snippet: "alpha evidence source",
          locator: { page: 1 },
        },
      ],
    });
  });

  it("preserves Auto dependency degradation metadata without citations", () => {
    const knowledge = normalizeMessageKnowledgeMetadata({
      knowledge: {
        mode: "auto",
        outcome: "dependency_unavailable",
        selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
        citationCount: 0,
        evidenceUsed: false,
        degradationReason: "dependency_unavailable",
      },
    });

    expect(knowledge).toMatchObject({
      mode: "auto",
      outcome: "dependency_unavailable",
      citationCount: 0,
      evidenceUsed: false,
      degradationReason: "dependency_unavailable",
      citations: [],
    });
  });

  it("drops stored citations that the completed answer did not use", () => {
    const knowledge = normalizeMessageKnowledgeMetadata(
      {
        knowledge: {
          mode: "auto",
          outcome: "answered",
          selectedCollectionIds: ["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],
          citationCount: 1,
          evidenceUsed: true,
          citations: [
            {
              id: "cit_1",
              marker: "[K1]",
              snippet: "related but non-answering evidence",
            },
          ],
        },
      },
      "The answer only used a public source [W1].",
    );

    expect(knowledge).toMatchObject({
      outcome: "answered_without_knowledge",
      citationCount: 0,
      evidenceUsed: false,
      citations: [],
    });
  });

  it("keeps only Knowledge markers present in the completed answer", () => {
    const knowledge = normalizeMessageKnowledgeMetadata(
      {
        knowledge: {
          mode: "auto",
          outcome: "answered",
          citationCount: 2,
          citations: [
            { id: "cit_1", marker: "[K1]", snippet: "first evidence" },
            { id: "cit_2", marker: "[K2]", snippet: "second evidence" },
          ],
        },
      },
      "The answer uses the second source [K2].",
    );

    expect(knowledge).toMatchObject({
      outcome: "answered",
      citationCount: 1,
      evidenceUsed: true,
      citations: [{ marker: "[K2]" }],
    });
  });

  it("removes a model-invented marker when the turn has no Knowledge evidence", () => {
    const knowledge = normalizeMessageKnowledgeMetadata(
      {
        knowledge: {
          outcome: "no_evidence",
          citationCount: 0,
          citations: [],
        },
      },
      "General answer [K1]",
    );

    expect(
      reconcileMessageKnowledgeContent("General answer [K1]", knowledge),
    ).toBe("General answer");
  });

  it("preserves only markers backed by current message metadata", () => {
    const knowledge = normalizeMessageKnowledgeMetadata(
      {
        knowledge: {
          outcome: "answered",
          citationCount: 1,
          citations: [
            {
              id: "cit_1",
              marker: "[K1]",
              snippet: "grounded fixture",
            },
          ],
        },
      },
      "Grounded [K1] invented [K2]",
    );

    expect(
      reconcileMessageKnowledgeContent(
        "Grounded [K1] invented [K2]",
        knowledge,
      ),
    ).toBe("Grounded [K1] invented");
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

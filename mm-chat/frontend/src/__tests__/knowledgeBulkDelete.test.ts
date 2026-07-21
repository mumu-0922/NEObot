import { describe, expect, it, vi } from "vitest";
import { deleteKnowledgeDocumentsWithConcurrency } from "@/lib/knowledge/bulkDelete";

describe("Knowledge document bulk deletion", () => {
  it("never exceeds the configured concurrency and preserves result order", async () => {
    let active = 0;
    let maxActive = 0;

    const result = await deleteKnowledgeDocumentsWithConcurrency({
      documentIds: ["doc-1", "doc-2", "doc-3", "doc-4", "doc-5"],
      concurrency: 3,
      deleteDocument: async () => {
        active += 1;
        maxActive = Math.max(maxActive, active);
        await new Promise((resolve) => setTimeout(resolve, 5));
        active -= 1;
      },
    });

    expect(maxActive).toBe(3);
    expect(result).toEqual({
      succeededIds: ["doc-1", "doc-2", "doc-3", "doc-4", "doc-5"],
      failedIds: [],
    });
  });

  it("continues after failures and reports failed ids in input order", async () => {
    const onDeleted = vi.fn();

    const result = await deleteKnowledgeDocumentsWithConcurrency({
      documentIds: ["doc-1", "doc-2", "doc-3", "doc-4"],
      concurrency: 2,
      deleteDocument: async (documentId) => {
        if (documentId === "doc-2" || documentId === "doc-4") {
          throw new Error("delete failed");
        }
      },
      onDeleted,
    });

    expect(result).toEqual({
      succeededIds: ["doc-1", "doc-3"],
      failedIds: ["doc-2", "doc-4"],
    });
    expect(onDeleted).toHaveBeenCalledTimes(2);
    expect(onDeleted).toHaveBeenCalledWith("doc-1");
    expect(onDeleted).toHaveBeenCalledWith("doc-3");
  });

  it("does not invoke the delete adapter for an empty selection", async () => {
    const deleteDocument = vi.fn<() => Promise<void>>();

    const result = await deleteKnowledgeDocumentsWithConcurrency({
      documentIds: [],
      concurrency: 3,
      deleteDocument,
    });

    expect(result).toEqual({ succeededIds: [], failedIds: [] });
    expect(deleteDocument).not.toHaveBeenCalled();
  });
});

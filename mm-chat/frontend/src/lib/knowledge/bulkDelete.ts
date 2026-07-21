export interface BulkDeleteKnowledgeDocumentsOptions {
  documentIds: string[];
  concurrency: number;
  deleteDocument: (documentId: string) => Promise<void>;
  onDeleted?: (documentId: string) => void;
}

export interface BulkDeleteKnowledgeDocumentsResult {
  succeededIds: string[];
  failedIds: string[];
}

export async function deleteKnowledgeDocumentsWithConcurrency({
  documentIds,
  concurrency,
  deleteDocument,
  onDeleted,
}: BulkDeleteKnowledgeDocumentsOptions): Promise<BulkDeleteKnowledgeDocumentsResult> {
  if (documentIds.length === 0) {
    return { succeededIds: [], failedIds: [] };
  }

  const succeededByIndex: Array<string | undefined> = new Array(
    documentIds.length,
  );
  const failedByIndex: Array<string | undefined> = new Array(
    documentIds.length,
  );
  const workerCount = Math.min(
    documentIds.length,
    Math.max(1, Math.floor(concurrency)),
  );
  let nextIndex = 0;

  const worker = async () => {
    while (nextIndex < documentIds.length) {
      const index = nextIndex;
      nextIndex += 1;
      const documentId = documentIds[index];

      try {
        await deleteDocument(documentId);
        succeededByIndex[index] = documentId;
        onDeleted?.(documentId);
      } catch {
        failedByIndex[index] = documentId;
      }
    }
  };

  await Promise.all(Array.from({ length: workerCount }, () => worker()));

  return {
    succeededIds: succeededByIndex.filter(
      (documentId): documentId is string => documentId !== undefined,
    ),
    failedIds: failedByIndex.filter(
      (documentId): documentId is string => documentId !== undefined,
    ),
  };
}

import type { KnowledgeCitation, MessageKnowledgeMetadata } from "./types";

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function stringArrayValue(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  const output: string[] = [];
  for (const item of value) {
    const id = stringValue(item);
    if (!id) continue;
    const key = id.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    output.push(id);
  }
  return output;
}

function normalizeKnowledgeCitation(value: unknown): KnowledgeCitation | null {
  if (!isRecord(value)) return null;
  const id = stringValue(value.id);
  const marker = stringValue(value.marker);
  const snippet = stringValue(value.snippet);
  if (!id || !marker || !snippet) return null;

  return {
    id,
    marker,
    snippet,
    collectionId: stringValue(value.collectionId),
    documentId: stringValue(value.documentId),
    documentVersionId: stringValue(value.documentVersionId),
    indexGenerationId: stringValue(value.indexGenerationId),
    materializationId: stringValue(value.materializationId),
    parentChunkId: stringValue(value.parentChunkId),
    childChunkId: stringValue(value.childChunkId),
    sourceSpanHash: stringValue(value.sourceSpanHash),
    contentHash: stringValue(value.contentHash),
    locator: value.locator,
    rankScore: numberValue(value.rankScore),
  };
}

export function normalizeMessageKnowledgeMetadata(
  metadata: Record<string, unknown> | undefined,
): MessageKnowledgeMetadata | undefined {
  if (!metadata || !isRecord(metadata.knowledge)) return undefined;
  const knowledge = metadata.knowledge;
  const rawCitations = Array.isArray(knowledge.citations)
    ? knowledge.citations
    : [];
  const citations = rawCitations
    .map(normalizeKnowledgeCitation)
    .filter((citation): citation is KnowledgeCitation => Boolean(citation));
  const citationCount =
    typeof knowledge.citationCount === "number" &&
    Number.isFinite(knowledge.citationCount)
      ? Math.max(0, Math.floor(knowledge.citationCount))
      : citations.length;

  return {
    mode: "auto",
    outcome: stringValue(knowledge.outcome),
    selectedCollectionIds: stringArrayValue(knowledge.selectedCollectionIds),
    citationCount,
    evidenceUsed:
      typeof knowledge.evidenceUsed === "boolean"
        ? knowledge.evidenceUsed
        : citations.length > 0
          ? true
          : undefined,
    degradationReason: stringValue(knowledge.degradationReason),
    citations,
  };
}

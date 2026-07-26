import type {
  KnowledgeCitation,
  KnowledgeCitationDisplayLocator,
  MessageKnowledgeMetadata,
} from "./types";

const reservedKnowledgeMarkerPattern = /[\t ]*\[K[0-9]+\]/g;
const maxKnowledgeSourceNameBytes = 512;
const maxKnowledgeDisplayCoordinate = 1_000_000_000;
const a1CellPattern = /^[A-Z]{1,4}[1-9][0-9]{0,9}$/;

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

function knowledgeSourceNameValue(value: unknown): string | undefined {
  const sourceName = stringValue(value);
  if (
    !sourceName ||
    /[\u0000-\u001f\u007f]/.test(sourceName) ||
    new TextEncoder().encode(sourceName).length > maxKnowledgeSourceNameBytes
  ) {
    return undefined;
  }
  return sourceName;
}

function displayCoordinate(value: unknown): number | undefined {
  return typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value > 0 &&
    value <= maxKnowledgeDisplayCoordinate
    ? value
    : undefined;
}

function displayCell(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const cell = value.trim().toUpperCase();
  return a1CellPattern.test(cell) ? cell : undefined;
}

function normalizeKnowledgeCitationDisplayLocator(
  value: unknown,
): KnowledgeCitationDisplayLocator | undefined {
  if (!isRecord(value)) return undefined;
  switch (value.kind) {
    case "page": {
      const page = displayCoordinate(value.page);
      return page ? { kind: "page", page } : undefined;
    }
    case "slide": {
      const slide = displayCoordinate(value.slide);
      return slide ? { kind: "slide", slide } : undefined;
    }
    case "cell_range": {
      const startCell = displayCell(value.startCell);
      const endCell = displayCell(value.endCell);
      return startCell && endCell
        ? { kind: "cell_range", startCell, endCell }
        : undefined;
    }
    case "line_range": {
      const startLine = displayCoordinate(value.startLine);
      const endLine = displayCoordinate(value.endLine);
      return startLine && endLine && endLine >= startLine
        ? { kind: "line_range", startLine, endLine }
        : undefined;
    }
    default:
      return undefined;
  }
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
    sourceName: knowledgeSourceNameValue(value.sourceName),
    displayLocator: normalizeKnowledgeCitationDisplayLocator(
      value.displayLocator,
    ),
    locator: value.locator,
    rankScore: numberValue(value.rankScore),
  };
}

export function normalizeMessageKnowledgeMetadata(
  metadata: Record<string, unknown> | undefined,
  answerContent?: string,
): MessageKnowledgeMetadata | undefined {
  if (!metadata || !isRecord(metadata.knowledge)) return undefined;
  const knowledge = metadata.knowledge;
  const reconcileWithAnswer = typeof answerContent === "string";
  const rawCitations = Array.isArray(knowledge.citations)
    ? knowledge.citations
    : [];
  const citations = rawCitations
    .map(normalizeKnowledgeCitation)
    .filter((citation): citation is KnowledgeCitation => {
      if (!citation) return false;
      return !reconcileWithAnswer || answerContent.includes(citation.marker);
    });
  const citationCount = reconcileWithAnswer
    ? citations.length
    : typeof knowledge.citationCount === "number" &&
        Number.isFinite(knowledge.citationCount)
      ? Math.max(0, Math.floor(knowledge.citationCount))
      : citations.length;
  const storedOutcome = stringValue(knowledge.outcome);
  const outcome =
    reconcileWithAnswer &&
    storedOutcome === "answered" &&
    citations.length === 0
      ? "answered_without_knowledge"
      : storedOutcome;

  return {
    mode: "auto",
    outcome,
    selectedCollectionIds: stringArrayValue(knowledge.selectedCollectionIds),
    citationCount,
    evidenceUsed: reconcileWithAnswer
      ? citations.length > 0
      : typeof knowledge.evidenceUsed === "boolean"
        ? knowledge.evidenceUsed
        : citations.length > 0
          ? true
          : undefined,
    degradationReason: stringValue(knowledge.degradationReason),
    citations,
  };
}

export function reconcileMessageKnowledgeContent(
  content: string,
  knowledge: MessageKnowledgeMetadata | undefined,
): string {
  if (!knowledge) return content;
  const allowed = new Set(
    (knowledge.citations || []).map((citation) => citation.marker),
  );
  return content.replace(reservedKnowledgeMarkerPattern, (match) =>
    allowed.has(match.trim()) ? match : "",
  );
}

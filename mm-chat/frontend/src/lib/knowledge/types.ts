export type KnowledgeCitationDisplayLocator =
  | { kind: "page"; page: number }
  | { kind: "slide"; slide: number }
  | {
      kind: "cell_range";
      startCell: string;
      endCell: string;
    }
  | {
      kind: "line_range";
      startLine: number;
      endLine: number;
    };

export interface KnowledgeCitation {
  id: string;
  marker: string;
  collectionId?: string;
  documentId?: string;
  documentVersionId?: string;
  indexGenerationId?: string;
  materializationId?: string;
  parentChunkId?: string;
  childChunkId?: string;
  sourceSpanHash?: string;
  contentHash?: string;
  sourceName?: string;
  displayLocator?: KnowledgeCitationDisplayLocator;
  locator?: unknown;
  snippet: string;
  rankScore?: number;
}

export interface MessageKnowledgeMetadata {
  mode: "auto";
  outcome?: string;
  selectedCollectionIds: string[];
  citationCount: number;
  evidenceUsed?: boolean;
  degradationReason?: string;
  citations: KnowledgeCitation[];
}

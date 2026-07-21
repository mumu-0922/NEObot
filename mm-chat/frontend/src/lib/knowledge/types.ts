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

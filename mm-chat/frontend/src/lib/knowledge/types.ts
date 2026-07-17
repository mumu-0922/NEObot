export type KnowledgeFileStatus =
  "uploading" | "parsing" | "indexing" | "indexed" | "saved" | "error";

export interface KnowledgeFile {
  id: string;
  name: string;
  size: number;
  type: string;
  uploadedAt: number;
  status: KnowledgeFileStatus;
  ragId?: string;
  ragChunkCount?: number;
  path?: string;
  error?: string;
}

export interface Collection {
  id: string;
  name: string;
  description: string;
  icon: string;
  color: string;
  files: KnowledgeFile[];
  updatedAt: number;
}

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

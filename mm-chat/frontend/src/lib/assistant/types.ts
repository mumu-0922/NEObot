export interface LobeAgentMeta {
  avatar: string;
  description: string;
  tags: string[];
  title: string;
  category: string;
  systemRole?: string;
}

export interface LobeAgent {
  identifier: string;
  meta: LobeAgentMeta;
  createdAt: string;
  homepage: string;
  author: string;
  isCustom?: boolean;
  updatedAt?: string;
  version?: string;
  fingerprint?: string;
  installCount?: number;
  isValidated?: boolean;
  safetyCheck?: string;
  requiredTools?: string[];
  admitted?: boolean;
  installed?: boolean;
  libraryId?: string;
}

export type AssistantSource = "custom" | "lobehub";

export interface AssistantLibraryEntry {
  id: string;
  source: AssistantSource;
  sourceIdentifier?: string;
  avatar: string;
  title: string;
  description: string;
  category: string;
  tags: string[];
  systemPrompt: string;
  author: string;
  homepage: string;
  sourceVersion?: string;
  sourceUpdatedAt?: string;
  requiredTools?: string[];
  contentFingerprint: string;
  revision: number;
  createdAt: string;
  updatedAt: string;
  updateAvailable?: boolean;
}

export interface AssistantMarketCategory {
  id: string;
  count: number;
}

export interface AssistantMarketSearchResult {
  agents: LobeAgent[];
  categories: AssistantMarketCategory[];
  page: number;
  pageSize: number;
  totalCount: number;
  totalPages: number;
  totalMarketCount?: number;
  source: string;
  unavailable?: boolean;
  canReview?: boolean;
}

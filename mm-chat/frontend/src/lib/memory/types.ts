export type MemoryType =
  | "fact"
  | "preference"
  | "instruction"
  | "project"
  | "warning"
  | "decision"
  | "context";

export type MemorySource = "manual" | "ai" | "direct_user" | "import" | "dream";

export type MemoryScopeType = "global" | "project" | "conversation";
export type MemoryPolicyMode = "inherit" | "on" | "off";
export type MemorySensitivity = "normal" | "sensitive";

export interface MemoryRecord {
  id: string;
  type: MemoryType;
  content: string;
  createdAt: number;
  updatedAt: number;
  lastUsedAt?: number;
  importance: number;
  tags: string[];
  source: MemorySource;
  sourceSessionId?: string;
  sourceMessageIds?: string[];
  sourceMemoryIds?: string[];
}

export interface MemorySettings {
  enabled: boolean;
  searchEnabled: boolean;
  autoRecordEnabled: boolean;
  dreamEnabled: boolean;
  triggerCount: number;
  targetCount: number;
}

export interface MemoryDreamStatus {
  isRunning: boolean;
  lastRunAt?: number;
  lastError?: string;
}

export interface MemoryProject {
  id: string;
  name: string;
  description: string;
  lifecycleStatus: "active" | "archived";
  revision: number;
  scopeGeneration: number;
  conversationCount: number;
  memoryCount: number;
  createdAt: number;
  updatedAt: number;
  archivedAt?: number;
}

export interface ConversationMemoryPolicy {
  conversationId: string;
  title: string;
  projectId?: string;
  projectName?: string;
  projectStatus?: "active" | "archived";
  useMode: MemoryPolicyMode;
  learnMode: MemoryPolicyMode;
  effectiveUse: boolean;
  effectiveLearn: boolean;
  learnForcedOff: boolean;
  scopeGeneration: number;
  updatedAt: number;
}

export interface GovernanceMemory {
  id: string;
  type: MemoryType;
  content: string;
  importance: number;
  tags: string[];
  source: MemorySource;
  authorityKind: "manual" | "direct_user" | "confirmed" | "import" | "auto";
  enabled: boolean;
  revision: number;
  scopeType: MemoryScopeType;
  projectId?: string;
  projectName?: string;
  conversationId?: string;
  conversationTitle?: string;
  lifecycleStatus: "active" | "superseded" | "expired" | "rejected";
  sensitivity: MemorySensitivity;
  recallStatus: string;
  validFrom?: number;
  validTo?: number;
  expiresAt?: number;
  supersededByMemoryId?: string;
  lastUsedAt?: number;
  createdAt: number;
  updatedAt: number;
}

export interface MemoryReviewTarget {
  memoryId: string;
  revision: number;
  type?: MemoryType;
  content?: string;
  scopeType?: MemoryScopeType;
  current: boolean;
}

export interface MemoryReviewSuggestion {
  id: string;
  type: MemoryType;
  content: string;
  importance: number;
  tags: string[];
  sensitivity: "normal" | "sensitive" | "secret";
  proposedAction: "ADD" | "NOOP" | "MERGE" | "SUPERSEDE" | "REJECT";
  reasonCode: string;
  scopeType: MemoryScopeType;
  projectId?: string;
  conversationId?: string;
  targets: MemoryReviewTarget[];
  evidenceMessageIds: string[];
  expiresAt: number;
  createdAt: number;
}

export interface MemoryReviewDecisionResult {
  suggestionId: string;
  decision:
    "keep_current" | "accept_new" | "edit_merge" | "keep_both" | "reject";
  status: "accepted" | "rejected";
  resultCode: string;
  memoryId?: string;
  memoryRevision?: number;
}

export interface MemoryDeletionProgress {
  manifestId: string;
  memoryId: string;
  immediateHidden: boolean;
  onlinePurgeStatus: "pending" | "online_purged";
  backupExpiryStatus: string;
  backupExpiresAt: number;
  deletedAt: number;
  purgedAt?: number;
}

export interface MemorySearchDiagnostic {
  assistantMessageId: string;
  profile: string;
  status: string;
  resultCode: string;
  fallbackCode: string;
  baselineCount: number;
  finalCount: number;
  overlapCount: number;
  estimatedTokens: number;
  durationMillis: number;
  createdAt: number;
}

export interface MemoryGovernanceSnapshot {
  settings: DurableMemoryGovernanceSettings;
  projects: MemoryProject[];
  conversations: ConversationMemoryPolicy[];
  memories: GovernanceMemory[];
  reviews: MemoryReviewSuggestion[];
  deletions: MemoryDeletionProgress[];
  diagnostics: MemorySearchDiagnostic[];
}

export interface DurableMemoryGovernanceSettings {
  enabled: boolean;
  searchEnabled: boolean;
  autoRecordEnabled: boolean;
  sensitiveMemoryEnabled: boolean;
  l2Mode: MemoryPolicyMode;
  l3Mode: MemoryPolicyMode;
}

export interface MemoryEvidence {
  messageId: string;
  conversationId: string;
  conversationTitle?: string;
  role: "user" | "assistant_context";
  sourceDeleted: boolean;
  sourceExcerpt?: string;
  observedAt: number;
}

export interface MemoryRevision {
  revision: number;
  operation: string;
  priorContent?: string;
  actorType: string;
  resultCode?: string;
  purged: boolean;
  createdAt: number;
}

export interface MemoryUsageLink {
  assistantMessageId: string;
  memoryRevision: number;
  createdAt: number;
}

export interface GovernanceMemoryDetail {
  memory: GovernanceMemory;
  evidence: MemoryEvidence[];
  history: MemoryRevision[];
  usages: MemoryUsageLink[];
}

export interface MemoryActivity {
  id: string;
  assistantMessageId: string;
  ordinal: number;
  subjectType: string;
  subjectId: string;
  subjectRevision?: number;
  action: string;
  status: string;
  reasonCode: string;
  undoKind: "none" | "created" | "corrected";
  undoStatus: "unavailable" | "available" | "undone" | "review_required";
  sourceKind: "direct_action" | "review_suggestion" | "memory_job";
  scopeType?: MemoryScopeType;
  memoryType?: MemoryType;
  memoryContent?: string;
  memoryRevision?: number;
  memoryDeleted?: boolean;
  createdAt: number;
  updatedAt: number;
}

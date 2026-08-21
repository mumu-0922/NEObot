import type { ImageSource, Source } from "../search/types";
import type { MessageKnowledgeMetadata } from "../knowledge/types";

export const IMAGE_CONTENT_POLICY_VIOLATION_CODE =
  "IMAGE_CONTENT_POLICY_VIOLATION";
export const IMAGE_PROVIDER_CONNECTION_CODE = "IMAGE_PROVIDER_CONNECTION_ERROR";
export const IMAGE_PROVIDER_TIMEOUT_CODE = "IMAGE_PROVIDER_TIMEOUT";
export const PROVIDER_ERROR_CODE = "PROVIDER_ERROR";
export const PROVIDER_STREAM_INTERRUPTED_CODE = "PROVIDER_STREAM_INTERRUPTED";

export interface Attachment {
  id: string;
  mimeType: string;
  data?: string;
  url?: string;
  fileName: string;
  source?: "server";
  fileId?: string;
  size?: number;
  sha256?: string;
  purpose?: string;
}

export type ProcessStepKind =
  "reasoning" | "knowledge" | "web" | "tool" | "generation";

export type ProcessStepStatus =
  | "pending"
  | "running"
  | "awaiting_approval"
  | "completed"
  | "failed"
  | "skipped"
  | "cancelled"
  | "outcome_unknown"
  | "interrupted";

export type ChatAgentEventType =
  | "turn.started"
  | "turn.ended"
  | "step.started"
  | "step.ended"
  | "assistant.message"
  | "tool.called"
  | "tool.result"
  | "goal.changed"
  | "goal.round.started"
  | "context.replaced"
  | "context.injected"
  | "assistant.chunk"
  | "assistant.block.completed";

export interface ChatAgentEvent {
  eventId: string;
  turnId: string;
  conversationId: string;
  messageId: string;
  runId: string;
  sequence: number;
  type: ChatAgentEventType;
  stepSequence?: number;
  payload: Record<string, unknown>;
  occurredAt: string;
}

export interface ProcessStep {
  id: string;
  kind: ProcessStepKind;
  status: ProcessStepStatus;
  labelKey: string;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  detail?: Record<string, unknown>;
  presentation?: ProcessStepPresentation;
}

export interface ProcessTranscriptEntry {
  sequence: number;
  stream: "stdout" | "stderr";
  content: string;
}

export interface ProcessPresentationItem {
  label: string;
  detail?: string;
}

export interface ProcessApprovalPresentation {
  id: string;
  revision: number;
  status: "pending" | "allowed" | "denied" | "expired";
  decision?:
    "allow_once" | "allow_conversation" | "deny" | "expired" | "restart_denied";
  expiresAt: string;
  allowConversation: boolean;
}

export interface ProcessRetryPresentation {
  eventId: string;
  retryOf: string;
}

export interface ProcessTerminalPresentation {
  version: 1;
  card: "terminal";
  command: string;
  cwd?: string;
  exitCode?: number;
  timedOut?: boolean;
  truncated?: boolean;
  background?: boolean;
  transcript?: ProcessTranscriptEntry[];
  approval?: ProcessApprovalPresentation;
}

export interface ProcessSearchPresentation {
  version: 1;
  card: "search";
  title?: string;
  provider?: string;
  query?: string;
  count?: number;
  summary?: string;
  items?: ProcessPresentationItem[];
  truncated?: boolean;
  approval?: ProcessApprovalPresentation;
}

export interface ProcessFilePresentation {
  version: 1;
  card: "file";
  operation?: string;
  path?: string;
  query?: string;
  summary?: string;
  content?: string;
  diff?: string;
  size?: number;
  offset?: number;
  nextOffset?: number;
  count?: number;
  items?: ProcessPresentationItem[];
  truncated?: boolean;
  title?: string;
  approval?: ProcessApprovalPresentation;
  retry?: ProcessRetryPresentation;
}

export interface ProcessJobPresentation {
  version: 1;
  card: "job";
  operation?: string;
  jobId?: string;
  jobStatus?: string;
  jobStartedAt?: string;
  jobCompletedAt?: string;
  jobDurationMs?: number;
  command?: string;
  cwd?: string;
  transcript?: ProcessTranscriptEntry[];
  exitCode?: number;
  timedOut?: boolean;
  truncated?: boolean;
  background?: boolean;
  approval?: ProcessApprovalPresentation;
}

export interface ProcessSummaryPresentation {
  version: 1;
  card: "skill" | "goal" | "mcp" | "browser";
  title?: string;
  summary?: string;
  operation?: string;
  items?: ProcessPresentationItem[];
  approval?: ProcessApprovalPresentation;
}

export type ProcessStepPresentation =
  | ProcessTerminalPresentation
  | ProcessSearchPresentation
  | ProcessFilePresentation
  | ProcessJobPresentation
  | ProcessSummaryPresentation;

export interface MessageVersion {
  id: string;
  content: string;
  reasoning?: string;
  timestamp: number;
  model: string;
  timing?: {
    startTime: number;
    endTime: number;
    duration: number;
  };
}

export interface ToolCall {
  id: string;
  name: string;
  args: any;
  status:
    | "pending"
    | "awaiting_confirmation"
    | "running"
    | "success"
    | "error"
    | "skipped"
    | "denied";
  result?: any;
  isError?: boolean;
  risk?: "read" | "write" | "destructive" | "external";
  confirmation?: {
    required: boolean;
    state: "pending" | "approved" | "denied";
    decidedAt?: number;
  };
  errorInfo?: {
    code?: string;
    message: string;
    recoverable?: boolean;
  };
  auth?: {
    type: "bearer" | "apiKey" | "none";
    value?: string;
    key?: string;
    addTo?: "header" | "query";
  };
}

export type MessageOutputBlock =
  | {
      id: string;
      type: "text";
      content: string;
    }
  | {
      id: string;
      type: "reasoning";
      content: string;
    }
  | {
      id: string;
      type: "search";
      isSearching?: boolean;
      error?: string;
      sources: Source[];
      images: ImageSource[];
    }
  | {
      id: string;
      type: "tool_group";
      toolCalls: ToolCall[];
    };

export interface Message {
  id: string;
  role: "user" | "model";
  content: string;
  parentMessageId?: string;
  treeParentMessageId?: string | null;
  reasoning?: string;
  processTrace?: ProcessStep[];
  agentEvents?: ChatAgentEvent[];
  timestamp: number;
  attachments?: Attachment[];
  toolCalls?: ToolCall[];
  legacySkillRetired?: true;
  model?: string;
  generationError?: {
    message: string;
    recoverable?: boolean;
    code?: string;
  };
  searchSources?: Source[];
  searchImages?: ImageSource[];
  isSearching?: boolean;
  outputBlocks?: MessageOutputBlock[];
  ragSources?: Source[];
  versions?: MessageVersion[];
  activeVersionId?: string;
  timing?: {
    startTime: number;
    endTime: number;
    duration: number;
  };
  usageMetadata?: {
    promptTokenCount: number;
    candidatesTokenCount: number;
    totalTokenCount: number;
  };
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
  suggestedQuestions?: string[];
  metadata?: Record<string, unknown>;
  knowledge?: MessageKnowledgeMetadata;
}

export interface MessageTreeNode {
  id: string;
  message: Message;
  parentMessageId?: string;
  childMessageIds: string[];
  activeChildMessageId?: string;
}

export interface SessionMessageTree {
  nodesById: Record<string, MessageTreeNode>;
  rootMessageIds: string[];
  activeRootMessageId?: string;
}

export type ChatPipelinePhase = "attachments" | "rag" | "search" | "model";

export type ChatPipelinePhaseState =
  "idle" | "running" | "success" | "warning" | "error";

export interface ChatPipelineStatus {
  phase: ChatPipelinePhase;
  state: ChatPipelinePhaseState;
  message?: string;
}

export interface ChatPipelineState {
  attachments: ChatPipelineStatus;
  rag: ChatPipelineStatus;
  search: ChatPipelineStatus;
  model: ChatPipelineStatus;
}

export type ChatGenerationStatus =
  | "idle"
  | "pending"
  | "attachments"
  | "rag"
  | "searching"
  | "tool"
  | "model"
  | "done"
  | "error"
  | "aborted";

export interface ChatGenerationState {
  status: ChatGenerationStatus;
  activeRunId?: number;
  sessionId?: string;
  userMessageId?: string;
  modelMessageId?: string;
  pipeline: ChatPipelineState;
  stopRequested: boolean;
  error?: {
    message: string;
    recoverable?: boolean;
    code?: string;
  };
}

export interface BackgroundTaskSnapshot {
  runId: number;
  sessionId: string;
  messageId: string;
  messageContent: string;
  sessionUpdatedAt?: number;
}

export type ChatGenerationEvent =
  | {
      type: "start";
      runId: number;
      sessionId: string;
      userMessageId: string;
    }
  | {
      type: "pipeline";
      runId: number;
      phase: ChatPipelinePhase;
      phaseState: ChatPipelinePhaseState;
      message?: string;
    }
  | {
      type: "optional-capability-failed";
      runId: number;
      phase: Exclude<ChatPipelinePhase, "model">;
      message: string;
    }
  | {
      type: "stream-started";
      runId: number;
      modelMessageId: string;
    }
  | { type: "stop-requested"; runId: number }
  | { type: "completed"; runId: number }
  | {
      type: "failed";
      runId: number;
      error: string;
      recoverable?: boolean;
      code?: string;
    }
  | { type: "aborted"; runId: number; reason?: string }
  | { type: "reset" };

export interface SessionConfig {
  toolMode?: ChatToolMode;
  searchMode?: SearchMode;
  useSearch?: boolean;
  searchResultsLimit?: number;
  useReasoning?: boolean;
  reasoningEffort?: ReasoningEffort;
  selectedKnowledgeCollectionIds?: string[];
}

export type ChatToolMode = "chat" | "agent";

export type SearchMode = "off" | "model_builtin" | "external";

export type ReasoningEffort =
  "auto" | "low" | "medium" | "high" | "xhigh" | "max";

export interface Session {
  id: string;
  title: string;
  messages?: Message[];
  messageCount: number;
  updatedAt: number;
  model: string;
  systemInstruction?: string;
  pinned?: boolean;
  workspaceId?: string;
  config?: SessionConfig;
  compression?: {
    compressedContent: string;
    lastCompressedMessageId: string;
  };
  memoryContext?: {
    injectedMemoryIds: string[];
    updatedAt?: number;
  };
}

export interface Workspace {
  id: string;
  name: string;
  systemPrompt?: string;
  files: Attachment[];
  color?: string;
  enableSearch?: boolean;
  enableReasoning?: boolean;
  createdAt: number;
  updatedAt?: number;
  revision?: number;
  bindingStatus?: "unbound" | "bound";
  runnerId?: string;
  canonicalPath?: string;
  displayPath?: string;
  pathKind?: "wsl" | "windows-mounted";
  directoryFingerprint?: string;
  boundAt?: number;
}

export interface Assistant {
  id: string;
  name: string;
  description: string;
  icon: string;
  systemInstruction?: string;
  color: string;
}

export interface ChatConfig {
  toolMode: ChatToolMode;
  searchMode: SearchMode;
  useSearch: boolean;
  useReasoning: boolean;
  reasoningEffort: ReasoningEffort;
  temperature: number;
}

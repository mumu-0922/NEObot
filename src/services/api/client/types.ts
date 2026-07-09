export type ApiMode = "local" | "server";

export type NetworkEdge = "same-origin-proxy" | "direct-cors";

export type UnsupportedFeatureCode =
  "FEATURE_NOT_IMPLEMENTED" | "SERVER_BASE_URL_REQUIRED";

export interface ApiClientEnv {
  NEXT_PUBLIC_API_MODE?: string;
  NEXT_PUBLIC_API_BASE_URL?: string;
}

export interface ApiClientConfig {
  mode?: string;
  baseUrl?: string;
  env?: ApiClientEnv;
  frontendOrigin?: string;
  networkEdge?: NetworkEdge;
}

export interface ResolvedApiClientConfig {
  mode: ApiMode;
  requestedMode: string;
  baseUrl: string;
  networkEdge: NetworkEdge;
  serverConfigured: boolean;
  warnings: string[];
}

export interface ApiCapabilities {
  chatCrud: boolean;
  chatStream: boolean;
  files: boolean;
  auth: boolean;
  imports: boolean;
  rag: boolean;
  plugins: boolean;
  providerSettings: boolean;
}

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    recoverable?: boolean;
    requestId?: string;
  };
}

export interface ApiPage<T> {
  items: T[];
  nextCursor?: string;
}

export interface ModelRef {
  providerId: string;
  modelId: string;
  displayName?: string;
}

export interface ConversationDTO {
  id: string;
  title: string;
  status: "active" | "archived" | "deleted";
  modelRef?: ModelRef;
  messageCount: number;
  config: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface ChatMessageDTO {
  id: string;
  conversationId: string;
  role: "system" | "user" | "assistant" | "tool";
  status: "pending" | "streaming" | "completed" | "failed" | "cancelled";
  content: string;
  sequenceNo: number;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  modelRef?: ModelRef;
  attachments: ServerAttachmentDTO[];
  outputBlocks: unknown[];
  metadata: Record<string, unknown>;
  parentMessageId?: string;
}

export interface ServerAttachmentDTO {
  id: string;
  source: "server";
  fileId: string;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: string;
  downloadUrl?: string;
}

export type FilePurpose =
  "chat" | "workspace" | "knowledge" | "image" | "audio" | "export";

export interface FileRecordDTO {
  id: string;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: FilePurpose;
  createdAt: string;
  downloadUrl: string;
}

export interface UploadFileInput {
  file: Blob;
  fileName?: string;
  purpose: FilePurpose;
  conversationId?: string;
  workspaceId?: string;
  knowledgeCollectionId?: string;
  clientFileId?: string;
  signal?: AbortSignal;
}

export interface DownloadFileContentInput {
  fileId: string;
  disposition?: "inline" | "attachment";
  signal?: AbortSignal;
}

export interface DownloadedFileContent {
  blob: Blob;
  contentType: string;
  size?: number;
}

export interface CreateConversationInput {
  title?: string;
  modelRef?: ModelRef;
  systemInstruction?: string;
  config?: Record<string, unknown>;
  idempotencyKey?: string;
}

export interface AppendUserMessageInput {
  conversationId: string;
  content: string;
  parentMessageId?: string;
  attachments?: Array<{
    source?: "server";
    fileId: string;
    purpose?: string;
  }>;
  metadata?: Record<string, unknown>;
  idempotencyKey?: string;
}

export interface StreamAssistantMessageInput {
  conversationId: string;
  userMessageId: string;
  modelRef: ModelRef;
  config?: Record<string, unknown>;
  systemInstruction?: string;
  systemPrompt?: string;
  metadata?: Record<string, unknown>;
  idempotencyKey: string;
  signal?: AbortSignal;
}

export interface ChatStreamHandlers {
  onStarted?: (event: ServerStreamEvent) => void;
  onDelta?: (event: ServerStreamEvent) => void;
  onUsage?: (event: ServerStreamEvent) => void;
  onCompleted?: (event: ServerStreamEvent) => void;
  onError?: (event: ServerStreamEvent) => void;
  onCancelled?: (event: ServerStreamEvent) => void;
}

export interface ChatRunResult {
  status: "completed" | "failed" | "cancelled" | "unsupported";
  message?: ChatMessageDTO;
  error?: ApiErrorEnvelope["error"];
}

export interface ChatApi {
  createConversation(input: CreateConversationInput): Promise<ConversationDTO>;
  listConversations(): Promise<ConversationDTO[]>;
  appendUserMessage(input: AppendUserMessageInput): Promise<ChatMessageDTO>;
  listMessages(conversationId: string): Promise<ChatMessageDTO[]>;
  streamAssistantMessage(
    input: StreamAssistantMessageInput,
    handlers?: ChatStreamHandlers,
  ): Promise<ChatRunResult>;
  cancelRun(runId: string): Promise<ChatRunResult>;
}

export interface FileApi {
  uploadFile(input: UploadFileInput): Promise<FileRecordDTO>;
  getFile(
    fileId: string,
    options?: { signal?: AbortSignal },
  ): Promise<FileRecordDTO>;
  downloadFileContent(
    input: DownloadFileContentInput,
  ): Promise<DownloadedFileContent>;
  deleteFile(fileId: string, options?: { signal?: AbortSignal }): Promise<void>;
}

export interface BrowserImportPackageInput {
  package: Blob;
  fileName?: string;
  signal?: AbortSignal;
}

export interface BrowserImportIssue {
  code: string;
  path: string;
  message: string;
  severity: "warning" | "error";
}

export interface BrowserImportPreviewResponse {
  summary: {
    conversations: number;
    messages: number;
    files: number;
    bytes: number;
    skippedDuplicates: number;
  };
  warnings: BrowserImportIssue[];
  errors: BrowserImportIssue[];
  commitAllowed: boolean;
}

export interface BrowserImportCommitResponse {
  batchId: string;
  status: "completed";
  created: {
    conversations: number;
    messages: number;
    files: number;
    attachments: number;
  };
  mappings: {
    conversations: Record<string, string>;
    messages: Record<string, string>;
    files: Record<string, string>;
  };
  warnings: BrowserImportIssue[];
}

export interface BrowserImportBatchStatus {
  batchId: string;
  status: "completed" | "rolled_back";
  createdAt: string;
}

export interface BrowserImportApi {
  previewBrowserImport(
    input: BrowserImportPackageInput,
  ): Promise<BrowserImportPreviewResponse>;
  commitBrowserImport(
    input: BrowserImportPackageInput,
  ): Promise<BrowserImportCommitResponse>;
  getBrowserImportBatch(
    batchId: string,
    options?: { signal?: AbortSignal },
  ): Promise<BrowserImportBatchStatus>;
  rollbackBrowserImportBatch(
    batchId: string,
    options?: { signal?: AbortSignal },
  ): Promise<void>;
}

export interface NeoChatApiClient {
  mode: ApiMode;
  config: ResolvedApiClientConfig;
  capabilities: ApiCapabilities;
  chat: ChatApi;
  files: FileApi;
  imports?: BrowserImportApi;
}

export type ServerStreamEventType =
  | "message.started"
  | "message.delta"
  | "usage.updated"
  | "message.completed"
  | "message.error"
  | "message.cancelled"
  | string;

export interface ServerStreamEvent {
  type: ServerStreamEventType;
  runId?: string;
  conversationId?: string;
  messageId?: string;
  sequence?: number;
  createdAt?: string;
  role?: "assistant";
  delta?: string;
  usage?: unknown;
  message?: ChatMessageDTO;
  error?: ApiErrorEnvelope["error"];
  [key: string]: unknown;
}

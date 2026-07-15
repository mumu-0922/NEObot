import type { ByokPublicKeyResponse } from "../../../lib/byok/shared";
import type { PublicServerConfig } from "../../../lib/defaultConfig/shared";

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
  agents: boolean;
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
  systemInstruction?: string;
  pinned?: boolean;
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

export interface UpdateConversationInput {
  conversationId: string;
  title?: string;
  modelRef?: ModelRef;
  systemInstruction?: string;
  config?: Record<string, unknown>;
  pinned?: boolean;
}

export interface DuplicateConversationInput {
  conversationId: string;
  title?: string;
  idempotencyKey?: string;
}

export interface GenerateConversationTitleInput {
  conversationId: string;
  modelRef?: ModelRef;
}

export interface GenerateConversationTitleResponse {
  title: string;
}

export interface GenerateRelatedQuestionsInput {
  conversationId: string;
  modelRef?: ModelRef;
}

export interface GenerateRelatedQuestionsResponse {
  questions: string[];
}

export interface DeleteMessageInput {
  conversationId: string;
  messageId: string;
  scope?: "message" | "subsequent";
}

export interface UpdateMessageInput {
  conversationId: string;
  messageId: string;
  content: string;
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

export interface ServerToolFunctionDefinition {
  name: string;
  description?: string;
  parameters: Record<string, unknown>;
}

export interface ServerToolDefinition {
  type: "function";
  function: ServerToolFunctionDefinition;
}

export interface PlanServerToolsInput {
  prompt: string;
  modelRef: ModelRef;
  tools: ServerToolDefinition[];
  signal?: AbortSignal;
}

export interface ServerPlannedToolCall {
  id: string;
  name: string;
  args: Record<string, unknown>;
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
  updateConversation(input: UpdateConversationInput): Promise<ConversationDTO>;
  deleteConversation(conversationId: string): Promise<void>;
  duplicateConversation(
    input: DuplicateConversationInput,
  ): Promise<ConversationDTO>;
  generateConversationTitle(
    input: GenerateConversationTitleInput,
  ): Promise<GenerateConversationTitleResponse>;
  generateRelatedQuestions(
    input: GenerateRelatedQuestionsInput,
  ): Promise<GenerateRelatedQuestionsResponse>;
  updateMessage(input: UpdateMessageInput): Promise<ChatMessageDTO>;
  deleteMessage(input: DeleteMessageInput): Promise<void>;
  appendUserMessage(input: AppendUserMessageInput): Promise<ChatMessageDTO>;
  listMessages(conversationId: string): Promise<ChatMessageDTO[]>;
  streamAssistantMessage(
    input: StreamAssistantMessageInput,
    handlers?: ChatStreamHandlers,
  ): Promise<ChatRunResult>;
  planTools(input: PlanServerToolsInput): Promise<ServerPlannedToolCall[]>;
  cancelRun(runId: string): Promise<ChatRunResult>;
}

export type AgentMarketLocale = "en" | "zh" | "ja";

export interface AgentListInput {
  locale?: AgentMarketLocale;
}

export interface AgentDetailInput {
  identifier: string;
  locale?: AgentMarketLocale;
}

export interface AgentListResponse {
  agents?: unknown[];
  unavailable?: boolean;
}

export interface AgentApi {
  listAgents(input?: AgentListInput): Promise<AgentListResponse>;
  getAgentDetail(input: AgentDetailInput): Promise<unknown>;
}

export type GlobalUserRole = "owner" | "user" | "viewer";

export interface AuthUserDTO {
  id: string;
  displayName: string;
  role: GlobalUserRole;
}

export interface LoginInput {
  email?: string;
  password: string;
}

export interface AcceptInviteInput {
  token: string;
  password: string;
}

export interface RecoveryRequestInput {
  email: string;
}

export interface CompleteRecoveryInput {
  token: string;
  newPassword: string;
}

export interface AuthenticatedRequestInput {
  token?: string;
}

export interface LoginResult {
  user: AuthUserDTO;
  token?: string;
  expiresAt?: string;
}

export interface AuthApi {
  getCurrentUser(
    input?: AuthenticatedRequestInput,
  ): Promise<AuthUserDTO | null>;
  login(input: LoginInput): Promise<LoginResult>;
  acceptInvite(input: AcceptInviteInput): Promise<LoginResult>;
  requestRecovery(input: RecoveryRequestInput): Promise<void>;
  completeRecovery(input: CompleteRecoveryInput): Promise<void>;
  logout(input?: AuthenticatedRequestInput): Promise<void>;
  revokeAllSessions(input?: AuthenticatedRequestInput): Promise<void>;
}

export interface SettingsApi {
  getRuntimeConfig(): Promise<PublicServerConfig>;
}

export interface ProviderRuntimeConfigDTO {
  type?: string;
  baseUrl?: string;
  name?: string;
  source?: "server-default";
  apiKeySecret?: unknown;
  useDefault?: boolean;
}

export interface ProviderModelsInput {
  provider: ProviderRuntimeConfigDTO;
  signal?: AbortSignal;
}

export interface ProviderModelsResponse {
  models: string[];
}

export interface ProviderApi {
  listModels(input: ProviderModelsInput): Promise<ProviderModelsResponse>;
}

export type BYOKPublicKeyResponse = ByokPublicKeyResponse;

export interface ByokApi {
  getPublicKey(): Promise<BYOKPublicKeyResponse>;
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
  auth: AuthApi;
  settings: SettingsApi;
  providers: ProviderApi;
  byok: ByokApi;
  chat: ChatApi;
  files: FileApi;
  imports?: BrowserImportApi;
  agents: AgentApi;
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

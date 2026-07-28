import type { ByokPublicKeyResponse } from "../../../lib/byok/shared";
import type { PublicServerConfig } from "../../../lib/defaultConfig/shared";
import type {
  ConversationMemoryPolicy,
  GovernanceMemory,
  GovernanceMemoryDetail,
  L2SceneGovernanceDetail,
  L2SceneGovernanceScene,
  L2SceneRebuildResult,
  MemoryActivity,
  MemoryDeletionProgress,
  MemoryGovernanceSnapshot,
  MemoryPolicyMode,
  MemoryProject,
  MemoryRecord,
  MemoryReviewDecisionResult,
  MemoryReviewSuggestion,
  MemoryScopeType,
  MemorySensitivity,
  MemoryType,
} from "../../../lib/memory/types";
import type {
  PluginExecutionPayload,
  PluginExecutionRequestPayload,
} from "../../../lib/plugin/execution";
import type { ProcessStep } from "../../../lib/chat/types";
import type { DefaultModels, Plugin } from "../../../types";

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
  teams: boolean;
  knowledge: boolean;
  memories: boolean;
  voice: boolean;
  voiceSynthesis: boolean;
  voiceTranscription: boolean;
  imageGeneration: boolean;
  codeExecution: boolean;
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

export interface CursorPageInput {
  cursor?: string;
  limit?: number;
  signal?: AbortSignal;
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

export interface ImportRemoteFileInput {
  url: string;
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

export interface GenerateTextInput {
  modelRef: ModelRef;
  provider?: ProviderRuntimeConfigDTO;
  prompt: string;
  signal?: AbortSignal;
}

export interface GenerateTextResponse {
  text: string;
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
  provider?: ProviderRuntimeConfigDTO;
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
  onReasoning?: (event: ServerStreamEvent) => void;
  onProcess?: (event: ServerStreamEvent) => void;
  onUsage?: (event: ServerStreamEvent) => void;
  onSearch?: (event: ServerStreamEvent) => void;
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
  generateText(input: GenerateTextInput): Promise<GenerateTextResponse>;
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
  getTaskModels(input?: { signal?: AbortSignal }): Promise<AdminTaskModelsDTO>;
  updateTaskModels(
    input: Partial<DefaultModels> & { signal?: AbortSignal },
  ): Promise<AdminTaskModelsDTO>;
}

export interface AdminTaskModelsDTO {
  models: DefaultModels;
  configured: boolean;
  updatedAt?: string;
}

export interface ProviderRuntimeConfigDTO {
  id?: string;
  type?: string;
  baseUrl?: string;
  name?: string;
  source?: "server-default" | "server-stored";
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

export interface AdminProviderConfigDTO {
  id: string;
  name: string;
  type: string;
  baseUrl: string;
  models: string[];
  enabled: boolean;
  hasApiKey: boolean;
  source: "server-default" | "server-stored";
  connectionTestValid: boolean;
  connectionTestedAt?: string;
  modelBuiltInSearch?: {
    protocol?: string;
    model?: string;
    source: "official" | "custom" | "none";
    connectionTestValid: boolean;
    connectionTestedAt?: string;
  };
  toolCapability?: {
    default?: string;
    modelOverrides?: Record<string, string>;
  };
}

export interface AdminProviderConfigsDTO {
  providers: AdminProviderConfigDTO[];
}

export interface UpdateAdminProviderConfigInput {
  name: string;
  type: string;
  baseUrl: string;
  models: string[];
  enabled?: boolean;
  apiKeySecret?: unknown;
  clearApiKey?: boolean;
  modelBuiltInSearchProtocol?: string;
  modelBuiltInSearchModel?: string;
  toolCapabilityDefault?: "auto" | "enabled" | "disabled";
  toolCapabilityModelOverrides?: Record<string, "enabled" | "disabled">;
  signal?: AbortSignal;
}

export interface AdminProviderConnectionDTO {
  provider: AdminProviderConfigDTO;
  models: string[];
}

export interface AdminModelBuiltInSearchConnectionDTO {
  provider: AdminProviderConfigDTO;
  sourceCount: number;
}

export interface ProviderApi {
  listModels(input: ProviderModelsInput): Promise<ProviderModelsResponse>;
  getServerDefaultConfig(): Promise<AdminProviderConfigDTO>;
  updateServerDefaultConfig(
    input: UpdateAdminProviderConfigInput,
  ): Promise<AdminProviderConfigDTO>;
  listAdminProviderConfigs(): Promise<AdminProviderConfigsDTO>;
  updateAdminProviderConfig(
    providerId: string,
    input: UpdateAdminProviderConfigInput,
  ): Promise<AdminProviderConfigDTO>;
  testAdminProviderConnection(
    providerId: string,
    signal?: AbortSignal,
  ): Promise<AdminProviderConnectionDTO>;
  activateAdminProvider(
    providerId: string,
    signal?: AbortSignal,
  ): Promise<AdminProviderConnectionDTO>;
  testAdminModelBuiltInSearch(
    providerId: string,
    input: { protocol: string; model: string; signal?: AbortSignal },
  ): Promise<AdminModelBuiltInSearchConnectionDTO>;
  deleteAdminProviderConfig(providerId: string): Promise<void>;
}

export type SearchProviderId = "tavily" | "firecrawl" | "exa" | "bocha";

export interface AdminSearchProviderConfigDTO {
  id: string;
  name: string;
  provider: SearchProviderId;
  baseUrl: string;
  enabled: boolean;
  hasApiKey: boolean;
  connectionTestValid: boolean;
  connectionTestedAt?: string;
}

export interface AdminSearchProviderConfigsDTO {
  providers: AdminSearchProviderConfigDTO[];
  activeProviderId?: SearchProviderId;
}

export interface UpdateAdminSearchProviderConfigInput {
  name: string;
  baseUrl: string;
  enabled?: boolean;
  apiKeySecret?: unknown;
  clearApiKey?: boolean;
  signal?: AbortSignal;
}

export interface AdminSearchProviderConnectionDTO {
  provider: AdminSearchProviderConfigDTO;
  sourceCount: number;
  imageCount: number;
}

export interface SearchProviderApi {
  listAdminSearchProviderConfigs(): Promise<AdminSearchProviderConfigsDTO>;
  updateAdminSearchProviderConfig(
    providerId: SearchProviderId,
    input: UpdateAdminSearchProviderConfigInput,
  ): Promise<AdminSearchProviderConfigDTO>;
  testAdminSearchProviderConnection(
    providerId: SearchProviderId,
    signal?: AbortSignal,
  ): Promise<AdminSearchProviderConnectionDTO>;
  activateAdminSearchProvider(
    providerId: SearchProviderId,
    signal?: AbortSignal,
  ): Promise<AdminSearchProviderConnectionDTO>;
  deleteAdminSearchProviderConfig(providerId: SearchProviderId): Promise<void>;
}

export type VoiceProviderId = "siliconflow";

export interface AdminVoiceProviderConfigDTO {
  id: string;
  name: string;
  provider: VoiceProviderId;
  baseUrl: string;
  model: string;
  voice: string;
  enabled: boolean;
  hasApiKey: boolean;
  connectionTestValid: boolean;
  connectionTestedAt?: string;
}

export interface AdminVoiceProviderConfigsDTO {
  providers: AdminVoiceProviderConfigDTO[];
  activeProviderId?: VoiceProviderId;
}

export interface UpdateAdminVoiceProviderConfigInput {
  enabled?: boolean;
  apiKeySecret?: unknown;
  clearApiKey?: boolean;
  signal?: AbortSignal;
}

export interface AdminVoiceProviderConnectionDTO {
  provider: AdminVoiceProviderConfigDTO;
  contentType: string;
  size: number;
}

export interface VoiceProviderApi {
  listAdminVoiceProviderConfigs(): Promise<AdminVoiceProviderConfigsDTO>;
  updateAdminVoiceProviderConfig(
    providerId: VoiceProviderId,
    input: UpdateAdminVoiceProviderConfigInput,
  ): Promise<AdminVoiceProviderConfigDTO>;
  testAdminVoiceProviderConnection(
    providerId: VoiceProviderId,
    signal?: AbortSignal,
  ): Promise<AdminVoiceProviderConnectionDTO>;
  activateAdminVoiceProvider(
    providerId: VoiceProviderId,
    signal?: AbortSignal,
  ): Promise<AdminVoiceProviderConnectionDTO>;
  deleteAdminVoiceProviderConfig(providerId: VoiceProviderId): Promise<void>;
}

export interface SynthesizeVoiceInput {
  messageId: string;
  text: string;
  signal?: AbortSignal;
}

export interface SynthesizedVoiceArtifactDTO {
  fileId: string;
  purpose: "audio";
  contentType: string;
  size: number;
  cached: boolean;
}

export interface VoiceJobApi {
  synthesizeVoice(
    input: SynthesizeVoiceInput,
  ): Promise<SynthesizedVoiceArtifactDTO>;
}

export type RAGProviderId = "mineru" | "siliconflow";

export interface AdminRAGProviderConfigDTO {
  id: string;
  name: string;
  provider: RAGProviderId;
  enabled: boolean;
  hasApiKey: boolean;
  connectionTestValid: boolean;
  connectionTestedAt?: string;
  embeddingModel?: string;
  embeddingDimensions?: number;
  rerankModel?: string;
  parserModel?: string;
}

export interface AdminRAGProviderConfigsDTO {
  providers: AdminRAGProviderConfigDTO[];
}

export type RAGServiceStatus = "ready" | "partial" | "unavailable";

export type RAGProviderRuntimeStatus =
  "ready" | "missing_secret" | "activation_required" | "unavailable";

export interface RAGProviderStateDTO {
  configured: boolean;
  status: RAGProviderRuntimeStatus;
  embeddingDimensions?: number;
}

export interface RAGProviderStatusDTO {
  providers: Record<RAGProviderId, RAGProviderStateDTO>;
  status: RAGServiceStatus;
  capabilities: {
    pdfParsing: boolean;
    nativeIndexing: boolean;
    retrieval: boolean;
  };
  ready: boolean;
}

export interface ConfigureAdminRAGProviderInput {
  apiKeySecret: unknown;
  signal?: AbortSignal;
}

export interface AdminRAGProviderConnectionDTO {
  provider: AdminRAGProviderConfigDTO;
  checks: string[];
}

export interface RAGProviderApi {
  listAdminRAGProviderConfigs(): Promise<AdminRAGProviderConfigsDTO>;
  getRAGProviderStatus(signal?: AbortSignal): Promise<RAGProviderStatusDTO>;
  configureAdminRAGProvider(
    providerId: RAGProviderId,
    input: ConfigureAdminRAGProviderInput,
  ): Promise<AdminRAGProviderConnectionDTO>;
  deleteAdminRAGProviderConfig(providerId: RAGProviderId): Promise<void>;
}

export interface GenerateImageInput {
  modelRef: ModelRef;
  prompt: string;
  size?: string;
  count?: number;
  signal?: AbortSignal;
}

export interface GeneratedImageArtifactDTO {
  fileId: string;
  purpose: "image";
  contentType: string;
  size: number;
}

export interface GenerateImageResponse {
  images: GeneratedImageArtifactDTO[];
  message: string;
}

export interface ImageGenerationApi {
  generateImage(input: GenerateImageInput): Promise<GenerateImageResponse>;
}

export type BYOKPublicKeyResponse = ByokPublicKeyResponse;

export interface ByokApi {
  getPublicKey(): Promise<BYOKPublicKeyResponse>;
}

export interface PluginListAvailableInput {
  signal?: AbortSignal;
}

export interface PluginListAvailableResponse {
  plugins: Plugin[];
  unavailable?: boolean;
}

export interface PluginInstallInput {
  plugin?: Plugin;
  customInput?: string;
  signal?: AbortSignal;
}

export interface PluginInstallResponse {
  plugin: Plugin;
}

export interface PluginExecuteInput {
  payload: PluginExecutionPayload | PluginExecutionRequestPayload;
  signal?: AbortSignal;
}

export interface PluginApi {
  listAvailable(
    input?: PluginListAvailableInput,
  ): Promise<PluginListAvailableResponse>;
  install(input: PluginInstallInput): Promise<PluginInstallResponse>;
  execute(input: PluginExecuteInput): Promise<Response>;
}

export interface FileApi {
  uploadFile(input: UploadFileInput): Promise<FileRecordDTO>;
  importRemoteFile(input: ImportRemoteFileInput): Promise<FileRecordDTO>;
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

export type TeamRole = "admin" | "member";
export type TeamMembershipStatus = "active" | "removed";
export type TeamInviteStatus = "pending" | "accepted" | "revoked" | "expired";
export type TeamInviteDeliveryStatus =
  "pending" | "processing" | "sent" | "failed" | "cancelled";

export interface TeamMembershipDTO {
  teamRole: TeamRole;
  status: TeamMembershipStatus;
  joinedAt: string;
  updatedAt: string;
}

export interface TeamDTO {
  id: string;
  name: string;
  membershipRevision: number;
  myMembership: TeamMembershipDTO;
  createdAt: string;
  updatedAt: string;
}

export interface TeamMemberDTO {
  userId: string;
  displayName: string;
  teamRole: TeamRole;
  status: TeamMembershipStatus;
  joinedAt: string;
  updatedAt: string;
}

export interface TeamInviteDTO {
  id: string;
  teamId: string;
  maskedEmail: string;
  teamRole: TeamRole;
  status: TeamInviteStatus;
  deliveryStatus: TeamInviteDeliveryStatus;
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreateTeamInput {
  name: string;
  idempotencyKey: string;
  signal?: AbortSignal;
}

export type ListTeamsInput = CursorPageInput;

export interface TeamLookupInput {
  teamId: string;
  signal?: AbortSignal;
}

export interface UpdateTeamInput extends TeamLookupInput {
  name: string;
}

export interface ListTeamMembersInput extends CursorPageInput {
  teamId: string;
}

export interface UpdateTeamMemberInput {
  teamId: string;
  userId: string;
  teamRole: TeamRole;
  signal?: AbortSignal;
}

export interface RemoveTeamMemberInput {
  teamId: string;
  userId: string;
  signal?: AbortSignal;
}

export type LeaveTeamInput = TeamLookupInput;

export interface CreateTeamInviteInput {
  teamId: string;
  email: string;
  teamRole: TeamRole;
  idempotencyKey: string;
  signal?: AbortSignal;
}

export interface ListTeamInvitesInput extends CursorPageInput {
  teamId: string;
}

export interface RevokeTeamInviteInput {
  teamId: string;
  inviteId: string;
  signal?: AbortSignal;
}

export interface TeamApi {
  createTeam(input: CreateTeamInput): Promise<TeamDTO>;
  listTeams(input?: ListTeamsInput): Promise<ApiPage<TeamDTO>>;
  getTeam(input: TeamLookupInput): Promise<TeamDTO>;
  updateTeam(input: UpdateTeamInput): Promise<TeamDTO>;
  listMembers(input: ListTeamMembersInput): Promise<ApiPage<TeamMemberDTO>>;
  leaveTeam(input: LeaveTeamInput): Promise<void>;
  updateMember(input: UpdateTeamMemberInput): Promise<TeamMemberDTO>;
  removeMember(input: RemoveTeamMemberInput): Promise<void>;
  createInvite(input: CreateTeamInviteInput): Promise<TeamInviteDTO>;
  listInvites(input: ListTeamInvitesInput): Promise<ApiPage<TeamInviteDTO>>;
  revokeInvite(input: RevokeTeamInviteInput): Promise<void>;
}

export type KnowledgeCollectionScope = "personal" | "team";
export type KnowledgeDocumentStatus = "processing" | "active" | "tombstoned";
export type KnowledgeDocumentVersionStatus =
  "uploaded" | "processing" | "failed" | "active" | "tombstoned";

export interface KnowledgePermissionsDTO {
  read: boolean;
  manage: boolean;
  manageConsent: boolean;
}

export interface KnowledgeCollectionDTO {
  id: string;
  name: string;
  description: string;
  icon: string;
  color: string;
  scope: KnowledgeCollectionScope;
  teamId?: string;
  permissions: KnowledgePermissionsDTO;
  aclRevision: number;
  visibilityEpoch: number;
  collectionProcessingRevision: number;
  createdAt: string;
  updatedAt: string;
}

export interface KnowledgeDocumentFileDTO {
  id: string;
  name: string;
  mimeType: string;
  byteSize: number;
}

export interface KnowledgeDocumentVersionDTO {
  id: string;
  sourceVersion: number;
  file: KnowledgeDocumentFileDTO;
  status: KnowledgeDocumentVersionStatus;
  createdAt: string;
  updatedAt: string;
  errorCode?: string;
}

export interface KnowledgeDocumentDTO {
  id: string;
  collectionId: string;
  status: KnowledgeDocumentStatus;
  currentVersion?: KnowledgeDocumentVersionDTO;
  pendingVersion?: KnowledgeDocumentVersionDTO;
  createdAt: string;
  updatedAt: string;
}

export interface ProcessingConsentDTO {
  processor: string;
  endpointId: string;
  modelId: string;
  profileContractHash: string;
  purposes: string[];
  dataTypes: string[];
  policyVersion: string;
  decision: string;
  effectiveStatus: string;
  expiresAt?: string;
  decidedAt: string;
}

export interface CreateKnowledgeCollectionInput {
  name: string;
  description?: string;
  icon?: string;
  color?: string;
  scope: KnowledgeCollectionScope;
  teamId?: string;
  idempotencyKey: string;
  signal?: AbortSignal;
}

export interface ListKnowledgeCollectionsInput extends CursorPageInput {
  scope?: KnowledgeCollectionScope;
  teamId?: string;
}

export interface KnowledgeCollectionLookupInput {
  collectionId: string;
  signal?: AbortSignal;
}

export interface UpdateKnowledgeCollectionInput extends KnowledgeCollectionLookupInput {
  name?: string;
  description?: string;
  icon?: string;
  color?: string;
}

export interface BindKnowledgeDocumentInput {
  collectionId: string;
  fileId: string;
  idempotencyKey: string;
  signal?: AbortSignal;
}

export interface ListKnowledgeDocumentsInput extends CursorPageInput {
  collectionId: string;
}

export interface KnowledgeDocumentLookupInput {
  documentId: string;
  signal?: AbortSignal;
}

export type DownloadKnowledgeDocumentContentInput =
  KnowledgeDocumentLookupInput;

export interface CreateKnowledgeDocumentVersionInput extends KnowledgeDocumentLookupInput {
  fileId: string;
  idempotencyKey: string;
}

export interface ReprocessKnowledgeDocumentInput extends KnowledgeDocumentLookupInput {
  idempotencyKey: string;
}

export type DeleteKnowledgeDocumentInput = KnowledgeDocumentLookupInput;

export interface ProcessingConsentIdentityInput {
  processor: string;
  endpointId?: string;
  modelId?: string;
}

export interface PutProcessingConsentInput extends ProcessingConsentIdentityInput {
  purposes: string[];
  dataTypes: string[];
  policyVersion: string;
  expiresAt?: string;
  signal?: AbortSignal;
}

export interface CollectionProcessingConsentInput extends ProcessingConsentIdentityInput {
  collectionId: string;
  signal?: AbortSignal;
}

export interface PutCollectionProcessingConsentInput extends PutProcessingConsentInput {
  collectionId: string;
}

export type PutQueryProcessingConsentInput = PutProcessingConsentInput;

export interface QueryProcessingConsentInput extends ProcessingConsentIdentityInput {
  signal?: AbortSignal;
}

export interface KnowledgeApi {
  createCollection(
    input: CreateKnowledgeCollectionInput,
  ): Promise<KnowledgeCollectionDTO>;
  listCollections(
    input?: ListKnowledgeCollectionsInput,
  ): Promise<ApiPage<KnowledgeCollectionDTO>>;
  getCollection(
    input: KnowledgeCollectionLookupInput,
  ): Promise<KnowledgeCollectionDTO>;
  updateCollection(
    input: UpdateKnowledgeCollectionInput,
  ): Promise<KnowledgeCollectionDTO>;
  deleteCollection(input: KnowledgeCollectionLookupInput): Promise<void>;
  bindDocument(
    input: BindKnowledgeDocumentInput,
  ): Promise<KnowledgeDocumentDTO>;
  listDocuments(
    input: ListKnowledgeDocumentsInput,
  ): Promise<ApiPage<KnowledgeDocumentDTO>>;
  getDocument(
    input: KnowledgeDocumentLookupInput,
  ): Promise<KnowledgeDocumentDTO>;
  downloadDocumentContent(
    input: DownloadKnowledgeDocumentContentInput,
  ): Promise<DownloadedFileContent>;
  createDocumentVersion(
    input: CreateKnowledgeDocumentVersionInput,
  ): Promise<KnowledgeDocumentDTO>;
  reprocessDocument(
    input: ReprocessKnowledgeDocumentInput,
  ): Promise<KnowledgeDocumentDTO>;
  deleteDocument(input: DeleteKnowledgeDocumentInput): Promise<void>;
  listCollectionConsents(
    input: KnowledgeCollectionLookupInput,
  ): Promise<ProcessingConsentDTO[]>;
  putCollectionConsent(
    input: PutCollectionProcessingConsentInput,
  ): Promise<ProcessingConsentDTO>;
  revokeCollectionConsent(
    input: CollectionProcessingConsentInput,
  ): Promise<void>;
  listQueryConsents(input?: {
    signal?: AbortSignal;
  }): Promise<ProcessingConsentDTO[]>;
  putQueryConsent(
    input: PutQueryProcessingConsentInput,
  ): Promise<ProcessingConsentDTO>;
  revokeQueryConsent(input: QueryProcessingConsentInput): Promise<void>;
}

export interface DurableMemorySettingsDTO {
  enabled: boolean;
  searchEnabled: boolean;
  autoRecordEnabled: boolean;
  sensitiveMemoryEnabled: boolean;
  l2Mode: MemoryPolicyMode;
  l3Mode: MemoryPolicyMode;
}

export interface MemoryMutationInput {
  type: MemoryType;
  content: string;
  importance?: number;
  tags?: string[];
  signal?: AbortSignal;
}

export interface UpdateMemoryInput extends MemoryMutationInput {
  memoryId: string;
}

export interface UpdateDurableMemorySettingsInput {
  enabled?: boolean;
  searchEnabled?: boolean;
  autoRecordEnabled?: boolean;
  sensitiveMemoryEnabled?: boolean;
  l2Mode?: MemoryPolicyMode;
  l3Mode?: MemoryPolicyMode;
  signal?: AbortSignal;
}

export interface CreateMemoryProjectInput {
  name: string;
  description?: string;
  signal?: AbortSignal;
}

export interface UpdateMemoryProjectInput extends CreateMemoryProjectInput {
  projectId: string;
  expectedRevision: number;
  lifecycleStatus: "active" | "archived";
}

export interface UpdateConversationMemoryPolicyInput {
  conversationId: string;
  expectedScopeGeneration: number;
  projectId?: string;
  useMode: MemoryPolicyMode;
  learnMode: MemoryPolicyMode;
  signal?: AbortSignal;
}

export interface GovernanceMemoryMutationInput {
  type: MemoryType;
  content: string;
  importance?: number;
  tags?: string[];
  scopeType: MemoryScopeType;
  projectId?: string;
  conversationId?: string;
  sensitivity?: MemorySensitivity;
  signal?: AbortSignal;
}

export interface UpdateGovernanceMemoryInput extends GovernanceMemoryMutationInput {
  memoryId: string;
  expectedRevision: number;
}

export interface SetL2SceneEnabledInput {
  sceneId: string;
  expectedRevision: number;
  enabled: boolean;
  signal?: AbortSignal;
}

export type MemoryImportPlanResult =
  "NOOP" | "ADD" | "REVIEW" | "REJECT" | "SCOPE_REQUIRED";

export interface MemoryImportProjectMapping {
  mode: "existing" | "create" | "skip";
  projectId?: string;
}

export interface MemoryImportConversationMapping {
  mode: "existing" | "global" | "project" | "skip";
  conversationId?: string;
  projectId?: string;
  projectRef?: string;
}

export interface MemoryImportMappings {
  projects: Record<string, MemoryImportProjectMapping>;
  conversations: Record<string, MemoryImportConversationMapping>;
}

export interface MemoryImportPlanItem {
  ordinal: number;
  memoryRef: string;
  recordHash: string;
  result: MemoryImportPlanResult;
  reasonCode: string;
  currentHash?: string;
}

export interface MemoryImportScopeRequirement {
  kind: "project" | "conversation";
  portableRef: string;
  name?: string;
  description?: string;
}

export interface MemoryImportDryRunResult {
  importId: string;
  packageSha256: string;
  manifestSha256: string;
  planSha256: string;
  planToken: string;
  expiresAt: number;
  counts: Record<MemoryImportPlanResult, number>;
  items: MemoryImportPlanItem[];
  scopeRequirements: MemoryImportScopeRequirement[];
  settingsSuggestion?: DurableMemorySettingsDTO;
}

export interface MemoryImportConfirmResult {
  importId: string;
  status: "completed";
  addedProjects: number;
  addedMemories: number;
  importedAt: number;
}

export interface MemoryImportPackageInput {
  packageFile: File;
  passphrase: string;
  mappings: MemoryImportMappings;
  signal?: AbortSignal;
}

export type MemoryReviewDecision =
  "keep_current" | "accept_new" | "edit_merge" | "keep_both" | "reject";

export interface MemoryActivityUndoResult {
  status: "undone" | "review_required";
  resultCode: string;
  memoryId?: string;
  memoryRevision?: number;
}

export interface MemoryApi {
  listMemories(input?: { signal?: AbortSignal }): Promise<MemoryRecord[]>;
  createMemory(input: MemoryMutationInput): Promise<MemoryRecord>;
  updateMemory(input: UpdateMemoryInput): Promise<MemoryRecord>;
  deleteMemory(input: {
    memoryId: string;
    signal?: AbortSignal;
  }): Promise<void>;
  getSettings(input?: {
    signal?: AbortSignal;
  }): Promise<DurableMemorySettingsDTO>;
  updateSettings(
    input: UpdateDurableMemorySettingsInput,
  ): Promise<DurableMemorySettingsDTO>;
  getGovernance(input?: {
    signal?: AbortSignal;
  }): Promise<MemoryGovernanceSnapshot>;
  listProjects(input?: { signal?: AbortSignal }): Promise<MemoryProject[]>;
  createProject(input: CreateMemoryProjectInput): Promise<MemoryProject>;
  updateProject(input: UpdateMemoryProjectInput): Promise<MemoryProject>;
  getConversationPolicy(input: {
    conversationId: string;
    signal?: AbortSignal;
  }): Promise<ConversationMemoryPolicy>;
  updateConversationPolicy(
    input: UpdateConversationMemoryPolicyInput,
  ): Promise<ConversationMemoryPolicy>;
  createGovernanceMemory(
    input: GovernanceMemoryMutationInput,
  ): Promise<GovernanceMemory>;
  updateGovernanceMemory(
    input: UpdateGovernanceMemoryInput,
  ): Promise<GovernanceMemory>;
  deleteGovernanceMemory(input: {
    memoryId: string;
    expectedRevision: number;
    signal?: AbortSignal;
  }): Promise<MemoryDeletionProgress>;
  getGovernanceMemoryDetail(input: {
    memoryId: string;
    signal?: AbortSignal;
  }): Promise<GovernanceMemoryDetail>;
  getL2SceneDetail(input: {
    sceneId: string;
    signal?: AbortSignal;
  }): Promise<L2SceneGovernanceDetail>;
  setL2SceneEnabled(
    input: SetL2SceneEnabledInput,
  ): Promise<L2SceneGovernanceScene>;
  rebuildL2Scene(input: {
    sceneId: string;
    signal?: AbortSignal;
  }): Promise<L2SceneRebuildResult>;
  rebuildL2Scenes(input?: {
    signal?: AbortSignal;
  }): Promise<L2SceneRebuildResult>;
  listMemoryReviews(input?: {
    signal?: AbortSignal;
  }): Promise<MemoryReviewSuggestion[]>;
  decideMemoryReview(input: {
    suggestionId: string;
    decision: MemoryReviewDecision;
    editedContent?: string;
    signal?: AbortSignal;
  }): Promise<MemoryReviewDecisionResult>;
  listMessageMemoryActivities(input: {
    assistantMessageId: string;
    limit?: number;
    signal?: AbortSignal;
  }): Promise<MemoryActivity[]>;
  undoMemoryActivity(input: {
    activityId: string;
    expectedRevision: number;
    signal?: AbortSignal;
  }): Promise<MemoryActivityUndoResult>;
  exportMemoryPackage(input: {
    passphrase: string;
    includeHistory: boolean;
    signal?: AbortSignal;
  }): Promise<Blob>;
  dryRunMemoryImport(
    input: MemoryImportPackageInput,
  ): Promise<MemoryImportDryRunResult>;
  confirmMemoryImport(
    input: MemoryImportPackageInput & { planToken: string },
  ): Promise<MemoryImportConfirmResult>;
}

export interface NeoChatApiClient {
  mode: ApiMode;
  config: ResolvedApiClientConfig;
  capabilities: ApiCapabilities;
  auth: AuthApi;
  settings: SettingsApi;
  providers: ProviderApi;
  searchProviders: SearchProviderApi;
  voiceProviders: VoiceProviderApi;
  ragProviders: RAGProviderApi;
  byok: ByokApi;
  images: ImageGenerationApi;
  voiceJobs: VoiceJobApi;
  chat: ChatApi;
  files: FileApi;
  plugins: PluginApi;
  imports?: BrowserImportApi;
  agents: AgentApi;
  teams: TeamApi;
  knowledge: KnowledgeApi;
  memories: MemoryApi;
}

export type ServerStreamEventType =
  | "message.started"
  | "message.delta"
  | "reasoning.delta"
  | "process.step.updated"
  | "usage.updated"
  | "search.results"
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
  step?: ProcessStep;
  usage?: unknown;
  results?: ServerSearchResult;
  message?: ChatMessageDTO;
  error?: ApiErrorEnvelope["error"];
  [key: string]: unknown;
}

export interface ServerSearchSource {
  title: string;
  url: string;
  content: string;
  metadata?: Record<string, unknown>;
}

export interface ServerSearchImage {
  url: string;
  description?: string;
}

export interface ServerSearchResult {
  sources: ServerSearchSource[];
  images: ServerSearchImage[];
}

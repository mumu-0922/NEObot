# Frontend API Client Contract

## 1. Purpose

This contract defines the stable boundary between the existing Next.js/React frontend and the future `mm-chat` server-backed stack. It is intentionally written before Go implementation so frontend, backend, storage, and migration work share one executable target.

The frontend must call domain clients, not raw backend routes, storage drivers, or provider SDKs from components.

```text
React components/hooks
  ↓
frontend API client contract
  ↓
local adapter OR server adapter
  ↓
IndexedDB/OPFS/Next.js routes OR Go/Postgres/MinIO/Redis/provider adapters
```

## 2. Source Inputs

This contract is derived from:

- `mm-chat/docs/architecture/server-refactor-design.md` Phase 2.
- `mm-chat/docs/inventory/api-routes.md` replacement API sketch.
- `mm-chat/docs/inventory/chat-flow.md` chat streaming spine.
- `mm-chat/docs/inventory/storage.md` localStorage/IndexedDB/OPFS inventory.
- `mm-chat/docs/inventory/provider-flow.md` server-side provider boundary.
- Existing plugin route inventory for deferred `pluginApi` shape.
- Existing service boundary in `src/services/README.md`.
- Existing chat types in `src/lib/chat/types.ts`.

## 3. Design Goals

- Preserve the current frontend and migrate behind a stable client interface.
- Support `local` mode and `server` mode through the same TypeScript contract.
- Make stream events explicit and provider-neutral.
- Keep provider secrets and object-storage keys out of browser-visible contracts.
- Make rollback a config switch, not a component rewrite.
- Keep Phase 2 as a contract only; implementation follows in later phases.

## 4. Non-Goals

- Rewriting React components in this phase.
- Implementing Go endpoints in this phase.
- Migrating browser data automatically.
- Moving plugin execution, code execution, voice, or full RAG into the first MVP. A minimal `pluginApi` contract is defined only to keep Phase 2 boundaries complete.
- Exposing database schemas directly to frontend code.

## 5. Runtime Modes

The client boundary must support two modes.

```ts
export type ApiMode = "local" | "server";
```

| Mode | Meaning | Backing Systems | Default |
|---|---|---|---|
| `local` | Preserve current app behavior | Zustand, localStorage, IndexedDB/localforage, OPFS, existing Next.js API routes | Yes during migration |
| `server` | Use new backend path | Go API, Postgres, Redis, MinIO, provider adapters | Opt-in per rollout |

Bootstrap configuration:

```env
NEXT_PUBLIC_API_MODE=local
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

Runtime configuration source:

```text
local mode   -> existing /api/config or local defaults
server mode  -> GET /v1/config from Go backend
```

Rules:

- `NEXT_PUBLIC_API_MODE` and `NEXT_PUBLIC_API_BASE_URL` are build-time defaults only.
- Runtime rollback must be driven by `/api/config` or `/v1/config` where possible; otherwise a Next.js rebuild/redeploy is required.
- Missing or invalid mode resolves to `local`.
- `server` mode requires a runtime or build-time API base URL.
- Mode selection happens in one factory, not per component.
- Mixed mode is allowed only through explicit capability flags, never by ad-hoc component logic.

## 6. Client Factory

Target module shape:

```text
src/services/api/client/
  index.ts
  types.ts
  local/
    chatApi.ts
    fileApi.ts
    authApi.ts
    settingsApi.ts
    providerApi.ts
  server/
    httpClient.ts
    chatApi.ts
    fileApi.ts
    authApi.ts
    settingsApi.ts
    providerApi.ts
```

Factory sketch:

```ts
export interface ApiClientConfig {
  mode?: ApiMode;
  baseUrl?: string;
  runtimeConfig?: RuntimeConfig;
  getAccessToken?: () => Promise<string | null> | string | null;
}

export interface NeoChatApiClient {
  mode: ApiMode;
  chat: ChatApi;
  files: FileApi;
  auth: AuthApi;
  settings: SettingsApi;
  providers: ProviderApi;
  plugins: PluginApi;
}

export function createNeoChatApiClient(config: ApiClientConfig): NeoChatApiClient;
```

The first implementation may live under `mm-chat/` as a prototype, but final integration should preserve the existing `src/services/api/*` import boundary until components are migrated safely.

## 7. Shared Data Types

The contract mirrors existing frontend concepts but avoids leaking local storage or DB internals.

```ts
export type EntityId = string;
export type IsoDateTime = string;

export interface ApiPage<T> {
  items: T[];
  nextCursor?: string;
}

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    requestId?: string;
    recoverable?: boolean;
    details?: unknown;
  };
}

export interface ModelRef {
  providerId: EntityId;
  modelId: string;
  displayName?: string;
}

export interface MessageVersionDto {
  id: EntityId;
  content: string;
  reasoning?: string;
  modelRef?: ModelRef;
  createdAt: IsoDateTime;
}

export type MessageOutputBlockDto =
  | { id: EntityId; type: "text"; content: string }
  | { id: EntityId; type: "reasoning"; content: string }
  | { id: EntityId; type: "search"; isSearching?: boolean; error?: string; sources: unknown[]; images: unknown[] }
  | { id: EntityId; type: "tool_group"; toolCalls: unknown[] }
  | { id: EntityId; type: "image"; attachments: AttachmentRef[] };
```

### 7.1 Conversation DTO

```ts
export interface ConversationSummary {
  id: EntityId;
  title: string;
  modelRef: ModelRef;
  messageCount: number;
  updatedAt: IsoDateTime;
  pinned?: boolean;
  workspaceId?: EntityId;
  config?: ConversationConfig;
}

export interface ConversationConfig {
  useSearch?: boolean;
  useReasoning?: boolean;
  useRag?: boolean;
  activePlugins?: string[];
  activeSkills?: string[];
}
```

Mapping notes:

- Current `Session.model` is a string that may encode `providerId:modelName`; API DTOs split this into `ModelRef.providerId` and `ModelRef.modelId` to avoid provider ambiguity.
- Current `Session.updatedAt` is a number; server DTO uses ISO strings at the API boundary.
- Current frontend can convert ISO strings to timestamps locally until components are refactored.
- Do not expose Postgres column names or server-only audit fields.

### 7.2 Message DTO

```ts
export type MessageRole = "user" | "assistant" | "system" | "tool";

export interface ChatMessageDto {
  id: EntityId;
  conversationId: EntityId;
  role: MessageRole;
  content: string;
  reasoning?: string;
  modelRef?: ModelRef;
  attachments?: AttachmentRef[];
  outputBlocks?: MessageOutputBlockDto[];
  usage?: TokenUsage;
  createdAt: IsoDateTime;
  updatedAt?: IsoDateTime;
  status?: "pending" | "streaming" | "completed" | "failed" | "cancelled";
  parentMessageId?: EntityId;
  childMessageIds?: EntityId[];
  activeChildMessageId?: EntityId;
  activeVersionId?: EntityId;
  versions?: MessageVersionDto[];
}
```

Compatibility notes:

- Existing frontend role `model` maps to server/API role `assistant`.
- Existing message branching/versioning is represented by optional tree/version fields; backend MVP may return empty fields but must not discard them during import or round-trip.
- Local adapter may keep current `Message` shape internally but must present this DTO at the client boundary.
- Tool calls stay opaque until plugin sandbox design is complete.

### 7.3 Attachment DTO

```ts
export type AttachmentRef =
  | {
      id: EntityId;
      source: "opfs";
      fileName: string;
      mimeType: string;
      url: `opfs://${string}`;
      size?: number;
      sha256?: string;
    }
  | {
      id: EntityId;
      source: "server";
      fileName: string;
      mimeType: string;
      fileId: EntityId;
      size?: number;
      sha256?: string;
    }
  | {
      id: EntityId;
      source: "remote";
      fileName: string;
      mimeType: string;
      url: string;
      size?: number;
    }
  | {
      id: EntityId;
      source: "inline";
      fileName: string;
      mimeType: string;
      data: string;
      size?: number;
      sha256?: string;
    };
```

Source matrix:

| Source | Allowed Mode | Required Field | Forbidden Field | Notes |
|---|---|---|---|---|
| `opfs` | `local` only | `url: opfs://...` | `fileId` | Never send raw `opfs://` URLs to Go except inside explicit import payloads. |
| `server` | `server` | `fileId` | MinIO bucket/key | Browser fetches through backend file API only. |
| `remote` | both, policy-gated | `url` | direct secret-bearing URLs | Backend must apply safe outbound policy before provider use. |
| `inline` | compatibility only | `data` | object storage keys | Use for small legacy/base64 payloads; migrate to files where possible. |

Rules:

- `local` mode may reference `opfs://` URLs internally.
- `server` mode accepts `fileId` for server-stored files, not MinIO object keys.
- Server downloads use backend-stream URLs (`/v1/files/:id/content`) unless a later architecture decision approves presigned URLs.
- Raw filenames are display data only; they must never become storage keys.
- Inline base64 attachments are allowed only as a temporary compatibility path.

### 7.4 Usage DTO

```ts
export interface TokenUsage {
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  providerPayload?: unknown;
}
```

## 8. `chatApi` Contract

```ts
export interface ChatApi {
  createConversation(input: CreateConversationInput): Promise<ConversationSummary>;
  listConversations(input?: ListConversationsInput): Promise<ApiPage<ConversationSummary>>;
  getConversation(id: EntityId): Promise<ConversationSummary>;
  updateConversation(id: EntityId, patch: UpdateConversationInput): Promise<ConversationSummary>;
  deleteConversation(id: EntityId): Promise<void>;

  listMessages(conversationId: EntityId, input?: ListMessagesInput): Promise<ApiPage<ChatMessageDto>>;
  appendUserMessage(conversationId: EntityId, input: AppendUserMessageInput): Promise<ChatMessageDto>;
  streamAssistantMessage(input: StreamAssistantMessageInput, handlers: ChatStreamHandlers): Promise<ChatRunResult>;
  cancelRun(runId: EntityId): Promise<CancelRunResult>;

  generateTitle(input: GenerateTitleInput): Promise<{ title: string }>;
}
```

### 8.1 Inputs

```ts
export interface CreateConversationInput {
  title?: string;
  modelRef: ModelRef;
  systemInstruction?: string;
  workspaceId?: EntityId;
  config?: ConversationConfig;
}

export interface ListConversationsInput {
  cursor?: string;
  limit?: number;
  pinned?: boolean;
  workspaceId?: EntityId;
}

export interface UpdateConversationInput {
  title?: string;
  pinned?: boolean;
  modelRef?: ModelRef;
  systemInstruction?: string | null;
  config?: ConversationConfig;
}

export interface ListMessagesInput {
  cursor?: string;
  limit?: number;
  afterMessageId?: EntityId;
}

export interface AppendUserMessageInput {
  content: string;
  attachments?: AttachmentRef[];
  skillInvocationIds?: EntityId[];
  clientMessageId?: EntityId;
}

export interface StreamAssistantMessageInput {
  conversationId: EntityId;
  userMessageId: EntityId;
  modelRef: ModelRef;
  config?: ConversationConfig & { temperature?: number };
  systemInstruction?: string;
  idempotencyKey: string;
}

export interface GenerateTitleInput {
  conversationId: EntityId;
  fallbackContent?: string;
}
```

Rules:

- `appendUserMessage` persists a user message without starting generation.
- Server-mode `streamAssistantMessage` requires a persisted `userMessageId`; callers must first use `appendUserMessage` for new user text.
- Server-mode `appendUserMessage` may link only `source: "server"`
  attachments that were already uploaded through `fileApi.upload`. The server
  adapter sends only `{ source: "server", fileId, purpose? }`; it must not send
  `opfs://` URLs, inline base64 data, remote URLs, object keys, or bucket names
  to the Go message endpoint.
- Server-mode attachment purpose defaults to `input`; `chat` maps to `input`
  and `knowledge` maps to `knowledge_source` for compatibility.
- Local mode may internally keep its existing one-step send behavior, but the server adapter must expose the two-step contract.
- `idempotencyKey` is required at the contract boundary. If a caller does not provide one, the adapter must generate it before making a server request.
- Local adapter may ignore pagination initially but must keep the method shape.

### 8.2 Server Endpoint Mapping

| `chatApi` Method | Server Endpoint | Notes |
|---|---|---|
| `createConversation` | `POST /v1/chat/conversations` | Creates metadata row |
| `listConversations` | `GET /v1/chat/conversations` | Cursor pagination |
| `getConversation` | `GET /v1/chat/conversations/:id` | Summary only |
| `updateConversation` | `PATCH /v1/chat/conversations/:id` | No message writes |
| `deleteConversation` | `DELETE /v1/chat/conversations/:id` | Soft delete preferred |
| `listMessages` | `GET /v1/chat/conversations/:id/messages` | Cursor pagination |
| `appendUserMessage` | `POST /v1/chat/conversations/:id/messages` | Role=user |
| `streamAssistantMessage` | `POST /v1/chat/conversations/:id/stream` | SSE response |
| `cancelRun` | `POST /v1/chat/runs/:runId/cancel` | Durable cancellation now; Redis flag later |
| `generateTitle` | `POST /v1/chat/conversations/:id/title` | Later helper; not first MVP |

## 9. Streaming Contract

Server mode uses `text/event-stream`. Local mode may simulate the same handler calls from existing streams.

```ts
export type ChatStreamEvent =
  | MessageStartedEvent
  | MessageDeltaEvent
  | MessageReasoningDeltaEvent
  | ToolCallEvent
  | ToolResultEvent
  | SearchUpdatedEvent
  | ImageGeneratedEvent
  | TimingUpdatedEvent
  | UsageUpdatedEvent
  | MessageCompletedEvent
  | MessageErrorEvent
  | MessageCancelledEvent;
```

### 9.1 Event Envelope

Every event has a stable envelope:

```ts
export interface StreamEventBase {
  type: string;
  runId: EntityId;
  conversationId: EntityId;
  messageId?: EntityId;
  sequence: number;
  createdAt: IsoDateTime;
}
```

`sequence` is monotonic per `runId`. The frontend must ignore duplicate sequence numbers and treat gaps as recoverable stream errors until the backend supports resume.

### 9.2 Events

```ts
export interface MessageStartedEvent extends StreamEventBase {
  type: "message.started";
  messageId: EntityId;
  role: "assistant";
  modelRef: ModelRef;
}

export interface MessageDeltaEvent extends StreamEventBase {
  type: "message.delta";
  messageId: EntityId;
  delta: string;
}

export interface MessageReasoningDeltaEvent extends StreamEventBase {
  type: "message.reasoning_delta";
  messageId: EntityId;
  delta: string;
}

export interface ToolCallEvent extends StreamEventBase {
  type: "tool.call";
  toolCall: unknown;
}

export interface ToolResultEvent extends StreamEventBase {
  type: "tool.result";
  toolResult: unknown;
}

export interface SearchUpdatedEvent extends StreamEventBase {
  type: "search.updated";
  isSearching: boolean;
  sources: unknown[];
  images: unknown[];
}

export interface ImageGeneratedEvent extends StreamEventBase {
  type: "image.generated";
  attachments: AttachmentRef[];
}

export interface TimingUpdatedEvent extends StreamEventBase {
  type: "timing.updated";
  startTime?: IsoDateTime;
  endTime?: IsoDateTime;
  durationMs?: number;
}

export interface UsageUpdatedEvent extends StreamEventBase {
  type: "usage.updated";
  usage: TokenUsage;
}

export interface MessageCompletedEvent extends StreamEventBase {
  type: "message.completed";
  messageId: EntityId;
  message: ChatMessageDto;
}

export interface MessageErrorEvent extends StreamEventBase {
  type: "message.error";
  error: ApiErrorEnvelope["error"];
}

export interface MessageCancelledEvent extends StreamEventBase {
  type: "message.cancelled";
  reason?: string;
}
```

### 9.3 Wire Examples

Canonical SSE wire examples:

```text
event: message.started
data: {"type":"message.started","runId":"run_1","conversationId":"c_1","messageId":"m_2","sequence":1,"createdAt":"2026-07-07T10:00:00.000Z","role":"assistant","modelRef":{"providerId":"provider_openai","modelId":"gpt-5.5","displayName":"GPT-5.5"}}

event: message.delta
data: {"type":"message.delta","runId":"run_1","conversationId":"c_1","messageId":"m_2","sequence":2,"createdAt":"2026-07-07T10:00:01.000Z","delta":"hello"}

event: message.completed
data: {"type":"message.completed","runId":"run_1","conversationId":"c_1","messageId":"m_2","sequence":3,"createdAt":"2026-07-07T10:00:02.000Z","message":{"id":"m_2","conversationId":"c_1","role":"assistant","content":"hello","createdAt":"2026-07-07T10:00:02.000Z"}}
```

Error frame:

```text
event: message.error
data: {"type":"message.error","runId":"run_1","conversationId":"c_1","sequence":4,"createdAt":"2026-07-07T10:00:03.000Z","error":{"code":"PROVIDER_TIMEOUT","message":"Provider timed out.","recoverable":true,"requestId":"req_1"}}
```

Framing rules:

- Use named `event:` lines matching `data.type`.
- Send one JSON object per event.
- Terminal events are `message.completed`, `message.error`, and `message.cancelled`.
- Optional feature events (`search.updated`, `image.generated`, `tool.*`) are emitted only when the matching capability is enabled.

### 9.4 Handler Interface

```ts
export interface ChatStreamHandlers {
  onStarted?: (event: MessageStartedEvent) => void;
  onDelta?: (event: MessageDeltaEvent) => void;
  onReasoningDelta?: (event: MessageReasoningDeltaEvent) => void;
  onToolCall?: (event: ToolCallEvent) => void;
  onToolResult?: (event: ToolResultEvent) => void;
  onSearch?: (event: SearchUpdatedEvent) => void;
  onImage?: (event: ImageGeneratedEvent) => void;
  onTiming?: (event: TimingUpdatedEvent) => void;
  onUsage?: (event: UsageUpdatedEvent) => void;
  onCompleted?: (event: MessageCompletedEvent) => void;
  onError?: (event: MessageErrorEvent) => void;
  onCancelled?: (event: MessageCancelledEvent) => void;
  signal?: AbortSignal;
}

export interface ChatRunResult {
  runId: EntityId;
  messageId?: EntityId;
  status: "completed" | "cancelled" | "failed";
  finalMessage?: ChatMessageDto;
}

export interface CancelRunResult {
  runId: EntityId;
  status: "cancelled";
  message: ChatMessageDto;
}
```

### 9.5 Compatibility With Current Stream Chunks

Current chunks use event data such as `content`, `reasoning`, `tool_call`, `tool_result`, and usage payloads. The adapter mapping is:

| Current Chunk | New Event |
|---|---|
| `content` | `message.delta` |
| `reasoning` | `message.reasoning_delta` |
| `tool_call` | `tool.call` |
| `tool_result` | `tool.result` |
| search status/results | `search.updated` |
| generated images | `image.generated` |
| timing metadata | `timing.updated` |
| usage payload | `usage.updated` |
| terminal success | `message.completed` |
| error chunk or thrown error | `message.error` |

## 10. `fileApi` Contract

```ts
export interface FileApi {
  upload(input: UploadFileInput): Promise<FileRecord>;
  getMetadata(fileId: EntityId): Promise<FileRecord>;
  getContent(fileId: EntityId, input?: GetFileContentInput): Promise<Blob>;
  getObjectUrl(fileId: EntityId): Promise<string>;
  delete(fileId: EntityId): Promise<void>;
}

export interface UploadFileInput {
  file: File;
  purpose: "chat" | "workspace" | "knowledge" | "image" | "audio" | "export";
  conversationId?: EntityId;
  workspaceId?: EntityId;
  knowledgeCollectionId?: EntityId;
  clientFileId?: EntityId;
}

export interface FileRecord {
  id: EntityId;
  fileName: string;
  mimeType: string;
  size: number;
  sha256?: string;
  purpose: UploadFileInput["purpose"];
  createdAt: IsoDateTime;
  downloadUrl?: string; // backend-stream URL only; no MinIO direct URL in MVP
}

export interface GetFileContentInput {
  disposition?: "inline" | "attachment";
}
```

Rules:

- Server mode maps to `POST /v1/files`, `GET /v1/files/:id`, `GET /v1/files/:id/content`, `DELETE /v1/files/:id`.
- Server mode never exposes MinIO bucket names, `storage_key`, or direct object-store URLs.
- Local mode wraps OPFS helpers and may return object URLs generated in the browser.
- Upload validation starts in the client for UX but is authoritative on the backend.

## 11. `authApi` Contract

First MVP may run as single-user or access-gated, but the client contract should not assume that forever.

```ts
export interface AuthApi {
  getCurrentUser(): Promise<CurrentUser | null>;
  login(input: LoginInput): Promise<CurrentUser>;
  logout(): Promise<void>;
}

export interface CurrentUser {
  id: EntityId;
  displayName: string;
  role: "owner" | "user" | "viewer";
  createdAt?: IsoDateTime;
}

export interface LoginInput {
  password?: string;
  token?: string;
}
```

Endpoint mapping:

| Method | Server Endpoint | Local Behavior |
|---|---|---|
| `getCurrentUser` | `GET /v1/me` | Returns synthetic local user |
| `login` | `POST /v1/auth/login` | Existing `/api/access/verify` compatibility |
| `logout` | `POST /v1/auth/logout` | Clears server/session state only when applicable |

Auth naming rule: server mode uses `login`; local mode may implement `login` by calling the existing access verification route until that route is replaced.

## 12. `settingsApi`, `providerApi`, and `pluginApi` Contracts

```ts
export interface SettingsApi {
  getRuntimeConfig(): Promise<RuntimeConfig>;
  getUserSettings(): Promise<UserSettingsSnapshot>;
  updateUserSettings(patch: Partial<UserSettingsSnapshot>): Promise<UserSettingsSnapshot>;
}

export interface ProviderApi {
  listProviders(): Promise<ProviderSummary[]>;
  listModels(input?: ListModelsInput): Promise<ModelInfo[]>;
}

export interface PluginApi {
  listAvailable(): Promise<PluginSummary[]>;
  listInstalled(): Promise<PluginSummary[]>;
  install(input: InstallPluginInput): Promise<PluginSummary>;
  execute(input: ExecutePluginInput): Promise<PluginExecutionResult>;
}

export interface RuntimeConfig {
  mode: ApiMode;
  serverVersion?: string;
  capabilities: CapabilityMap;
}

export interface CapabilityMap {
  serverChat: boolean;
  serverFiles: boolean;
  serverProviderSecrets: boolean;
  rag: boolean;
  plugins: boolean;
  voice: boolean;
  importLocalData: boolean;
}

export interface UserSettingsSnapshot {
  defaultModelRef?: ModelRef;
  language?: string;
  theme?: string;
  chatDefaults?: ConversationConfig;
}

export interface PluginSummary {
  id: EntityId;
  name: string;
  description?: string;
  installed: boolean;
  enabled: boolean;
  risk?: "low" | "medium" | "high";
}

export interface InstallPluginInput {
  source: "catalog" | "manifest";
  identifier?: string;
  manifest?: unknown;
}

export interface ExecutePluginInput {
  pluginId: EntityId;
  functionName: string;
  args: unknown;
  conversationId?: EntityId;
}

export interface PluginExecutionResult {
  ok: boolean;
  result?: unknown;
  error?: ApiErrorEnvelope["error"];
}

export interface ProviderSummary {
  id: EntityId;
  name: string;
  type: "OpenAI" | "OpenAICompatible" | "Gemini" | string;
  enabled: boolean;
  serverManaged: boolean;
}

export interface ListModelsInput {
  providerId?: EntityId;
  refresh?: boolean;
}

export interface ModelInfo {
  providerId: EntityId;
  modelId: string;
  name: string;
  displayName: string;
  description?: string;
  providerName?: string;
}
```

Endpoint mapping:

| Client Method | Server Endpoint | Notes |
|---|---|---|
| `settings.getRuntimeConfig` | `GET /v1/config` | Runtime mode/capability bootstrap; `/api/config` remains local compatibility. |
| `settings.getUserSettings` | `GET /v1/settings` | User-visible settings only; no plaintext provider secrets. |
| `settings.updateUserSettings` | `PATCH /v1/settings` | Partial update with server validation. |
| `providers.listProviders` | `GET /v1/providers` | Returns provider metadata and `serverManaged` flags. |
| `providers.listModels` | `POST /v1/providers/models` | Allows refresh and provider-specific lookup. |
| `plugins.listAvailable` | `GET /v1/plugins` | Later phase; capability-gated. |
| `plugins.listInstalled` | `GET /v1/plugins/installed` | Later phase; capability-gated. |
| `plugins.install` | `POST /v1/plugins/install` | Later phase; validate manifest before install. |
| `plugins.execute` | `POST /v1/plugins/execute` | Deferred until sandbox design exists. |

Rules:

- Server mode sends provider IDs/model IDs, not plaintext API keys.
- Local mode can keep existing BYOK behavior.
- `RuntimeConfig.capabilities` gates UI visibility for features not yet migrated.
- `plugins` capability remains `false` for the first server MVP; `pluginApi` exists to avoid later component-level route coupling.

## 13. HTTP Client Rules

Server adapter must centralize HTTP behavior.

```ts
export interface HttpClientOptions {
  baseUrl: string;
  credentials?: RequestCredentials;
  defaultHeaders?: Record<string, string>;
}
```

Rules:

- All server JSON requests send `Content-Type: application/json`.
- All responses include or synthesize `requestId` for error reporting.
- `AbortSignal` must be passed through for streaming and uploads.
- `401` maps to `AUTH_REQUIRED`.
- `403` maps to `FORBIDDEN`.
- `404` maps to entity-specific `*_NOT_FOUND` where possible.
- `409` maps to `CONFLICT` or idempotency conflict.
- `413` maps to `FILE_TOO_LARGE`.
- `429` maps to `RATE_LIMITED` and should preserve retry metadata when
  available: `Retry-After`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and
  `X-RateLimit-Reset`.
- `5xx` maps to `SERVER_ERROR` and is recoverable unless explicitly marked otherwise.

## 14. Error Matrix

| Condition | Code | Recoverable | Owner |
|---|---|---:|---|
| Missing/expired session | `AUTH_REQUIRED` | Yes | authApi/httpClient |
| No access to conversation/file | `FORBIDDEN` | No | backend |
| Conversation missing | `CONVERSATION_NOT_FOUND` | No | chatApi |
| Message missing | `MESSAGE_NOT_FOUND` | No | chatApi |
| File too large | `FILE_TOO_LARGE` | Yes after user action | fileApi/backend |
| Unsupported MIME | `UNSUPPORTED_FILE_TYPE` | Yes after user action | fileApi/backend |
| Provider not configured | `PROVIDER_NOT_CONFIGURED` | Yes after settings change | providerApi/chatApi |
| Provider timeout | `PROVIDER_TIMEOUT` | Yes | provider adapter |
| Stream interrupted | `STREAM_INTERRUPTED` | Yes | chatApi/httpClient |
| Rate limited | `RATE_LIMITED` | Yes after wait | backend/Redis |
| Invalid import/export payload | `INVALID_IMPORT_PAYLOAD` | No for current payload | import phase later |

## 15. Migration Sequence

| Step | Contract Work | Runtime Change | Rollback |
|---:|---|---|---|
| 1 | Add client interfaces and factory | No behavior change | Remove unused interfaces |
| 2 | Wrap current local chat service as `local.chatApi` | Components still use local path | Keep old imports |
| 3 | Add server HTTP adapter with health/config only | Can smoke test Go backend | Switch `NEXT_PUBLIC_API_MODE=local` |
| 4 | Add server conversation/message CRUD | Server persistence opt-in | Switch mode local |
| 5 | Add server stream events | SSE chat opt-in | Switch mode local |
| 6 | Add file API local adapter wrapper | No server files yet | Keep OPFS path |
| 7 | Add server file API | MinIO opt-in | Disable `serverFiles` capability |
| 8 | Add provider config/model APIs | Provider secrets server-side | Fall back to local BYOK |

## 16. Test Requirements

When implementation begins, add tests for:

- Mode resolution: invalid/missing mode returns `local`.
- Local adapter preserves current `streamChatResponse` behavior.
- Server adapter builds correct endpoints and parses error envelope.
- SSE parser maps every event type to the correct handler and accepts canonical `event:`/`data:` frames.
- AbortSignal cancels active stream and calls `cancelRun` when run ID exists.
- File upload rejects oversized files before network call and backend still enforces size.
- Provider/model mapping: legacy `providerId:modelName` strings split into `ModelRef` without ambiguity.
- Role mapping: local `model` ↔ API `assistant` round trip.
- Date mapping: ISO API dates ↔ frontend timestamps where legacy components require numbers.

## 17. Acceptance Criteria for Phase 2

- `frontend-api-client.md` defines stable `chatApi`, `fileApi`, `authApi`, `settingsApi`, and `providerApi` contracts.
- Contract includes local/server mode rules.
- Contract includes SSE event envelope and event types.
- Contract includes error envelope and error matrix.
- Contract maps first server endpoints back to Phase 1 inventories.
- Contract includes provider/model identity, runtime config rollback, attachment source matrix, and plugin placeholder boundaries.
- `progress.md` and `process.md` are updated after review.

## 18. Open Decisions for Later Phases

- Whether `server` mode initially supports anonymous single-user access or mandatory login.
- Whether BYOK remains available in hosted server mode or only local mode.
- Whether server file downloads use backend streaming only or later presigned URLs.
- Whether search/RAG helpers stay in chat API or move to dedicated APIs after MVP.
- Whether plugin/tool execution is disabled, client-only, or sandboxed server-side in the first public server release.

# Frontend API Client Contract

## 1. Purpose

This contract defines the stable boundary between the existing Next.js/React frontend and the future `mm-chat` server-backed stack. It is intentionally written before Go implementation so frontend, backend, storage, and migration work share one executable target.

The frontend must call domain clients, not raw backend routes, storage drivers, or provider SDKs from components.

Phase 11 uses the implementation addendum in
[§20](#20-phase-11-server-mode-integration-implementation-contract) as the
authoritative slice contract. Where §20 narrows a future endpoint or capability
listed earlier in this document, §20 wins for Phase 11.

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
- Implemented Go chat CRUD contract in
  [`chat-crud-api.md`](./chat-crud-api.md).
- Implemented Go SSE contract in
  [`chat-stream-api.md`](./chat-stream-api.md).
- Implemented Go file contract in [`file-api.md`](./file-api.md).

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

| Mode     | Meaning                       | Backing Systems                                                                 | Default              |
| -------- | ----------------------------- | ------------------------------------------------------------------------------- | -------------------- |
| `local`  | Preserve current app behavior | Zustand, localStorage, IndexedDB/localforage, OPFS, existing Next.js API routes | Yes during migration |
| `server` | Use new backend path          | Go API, Postgres, Redis, MinIO, provider adapters                               | Opt-in per rollout   |

Bootstrap configuration:

```env
NEXT_PUBLIC_API_MODE=local
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

Runtime configuration source:

```text
local mode   -> existing /api/config or local defaults
server mode  -> Phase 11: build-time env/static capabilities;
                future: GET /v1/config after a Go route exists
```

Rules:

- `NEXT_PUBLIC_API_MODE` and `NEXT_PUBLIC_API_BASE_URL` are build-time defaults only.
- Runtime rollback must be driven by `/api/config` or a future `/v1/config`
  where possible; otherwise a Next.js rebuild/redeploy is required.
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
    importApi.ts
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
  imports: ImportApi;
}

export function createNeoChatApiClient(
  config: ApiClientConfig,
): NeoChatApiClient;
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
  | {
      id: EntityId;
      type: "search";
      isSearching?: boolean;
      error?: string;
      sources: unknown[];
      images: unknown[];
    }
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

| Source   | Allowed Mode       | Required Field    | Forbidden Field            | Notes                                                                       |
| -------- | ------------------ | ----------------- | -------------------------- | --------------------------------------------------------------------------- |
| `opfs`   | `local` only       | `url: opfs://...` | `fileId`                   | Never send raw `opfs://` URLs to Go except inside explicit import payloads. |
| `server` | `server`           | `fileId`          | MinIO bucket/key           | Browser fetches through backend file API only.                              |
| `remote` | both, policy-gated | `url`             | direct secret-bearing URLs | Backend must apply safe outbound policy before provider use.                |
| `inline` | compatibility only | `data`            | object storage keys        | Use for small legacy/base64 payloads; migrate to files where possible.      |

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
  createConversation(
    input: CreateConversationInput,
  ): Promise<ConversationSummary>;
  listConversations(
    input?: ListConversationsInput,
  ): Promise<ApiPage<ConversationSummary>>;
  getConversation(id: EntityId): Promise<ConversationSummary>;
  updateConversation(
    id: EntityId,
    patch: UpdateConversationInput,
  ): Promise<ConversationSummary>;
  deleteConversation(id: EntityId): Promise<void>;

  listMessages(
    conversationId: EntityId,
    input?: ListMessagesInput,
  ): Promise<ApiPage<ChatMessageDto>>;
  appendUserMessage(
    conversationId: EntityId,
    input: AppendUserMessageInput,
  ): Promise<ChatMessageDto>;
  streamAssistantMessage(
    input: StreamAssistantMessageInput,
    handlers: ChatStreamHandlers,
  ): Promise<ChatRunResult>;
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

| `chatApi` Method         | Server Endpoint                            | Notes                                      |
| ------------------------ | ------------------------------------------ | ------------------------------------------ |
| `createConversation`     | `POST /v1/chat/conversations`              | Creates metadata row                       |
| `listConversations`      | `GET /v1/chat/conversations`               | Cursor pagination                          |
| `getConversation`        | `GET /v1/chat/conversations/:id`           | Summary only                               |
| `updateConversation`     | `PATCH /v1/chat/conversations/:id`         | No message writes                          |
| `deleteConversation`     | `DELETE /v1/chat/conversations/:id`        | Soft delete preferred                      |
| `listMessages`           | `GET /v1/chat/conversations/:id/messages`  | Cursor pagination                          |
| `appendUserMessage`      | `POST /v1/chat/conversations/:id/messages` | Role=user                                  |
| `streamAssistantMessage` | `POST /v1/chat/conversations/:id/stream`   | SSE response                               |
| `cancelRun`              | `POST /v1/chat/runs/:runId/cancel`         | Durable cancellation now; Redis flag later |
| `generateTitle`          | `POST /v1/chat/conversations/:id/title`    | Later helper; not first MVP                |

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

| Current Chunk               | New Event                 |
| --------------------------- | ------------------------- |
| `content`                   | `message.delta`           |
| `reasoning`                 | `message.reasoning_delta` |
| `tool_call`                 | `tool.call`               |
| `tool_result`               | `tool.result`             |
| search status/results       | `search.updated`          |
| generated images            | `image.generated`         |
| timing metadata             | `timing.updated`          |
| usage payload               | `usage.updated`           |
| terminal success            | `message.completed`       |
| error chunk or thrown error | `message.error`           |

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
  login(input: LoginInput): Promise<LoginResult>;
  acceptInvite(input: AcceptInviteInput): Promise<LoginResult>;
  requestRecovery(input: RecoveryRequestInput): Promise<void>;
  completeRecovery(input: RecoveryCompleteInput): Promise<void>;
  logout(): Promise<void>;
  revokeAllSessions(): Promise<void>;
}

export interface CurrentUser {
  id: EntityId;
  displayName: string;
  role: "owner" | "user" | "viewer";
  createdAt?: IsoDateTime;
}

export interface LoginInput {
  email: string;
  password: string;
}

export interface AcceptInviteInput {
  token: string;
  password: string;
}

export interface RecoveryRequestInput {
  email: string;
}

export interface RecoveryCompleteInput {
  token: string;
  newPassword: string;
}

export interface LoginResult {
  user: CurrentUser;
  token?: string; // server mode bearer token returned once
  expiresAt?: IsoDateTime;
}
```

Endpoint mapping:

| Method              | Server Endpoint                   | Local Behavior                                   |
| ------------------- | --------------------------------- | ------------------------------------------------ |
| `getCurrentUser`    | `GET /v1/me`                      | Returns synthetic local user                     |
| `login`             | `POST /v1/auth/login`             | Existing `/api/access/verify` compatibility      |
| `acceptInvite`      | `POST /v1/auth/invites/accept`    | Unsupported                                      |
| `requestRecovery`   | `POST /v1/auth/recovery/request`  | Unsupported                                      |
| `completeRecovery`  | `POST /v1/auth/recovery/complete` | Unsupported                                      |
| `logout`            | `POST /v1/auth/logout`            | Clears server/session state only when applicable |
| `revokeAllSessions` | `DELETE /v1/me/sessions`          | Unsupported                                      |

Auth naming rule: server mode uses `login`; local mode may implement `login` by calling the existing access verification route until that route is replaced.

Phase 15.1B supersedes the Phase 13 Bootstrap Token DTO. Server Login accepts
only `{ email, password }` and returns `{ user, token, expiresAt }`, where
`token` is the raw Bearer Session Token returned once. Invite Acceptance has
the same success shape. The client stores the Token as runtime Session state
and sends it as `Authorization: Bearer <token>` for server API calls. Recovery
Request always renders the same accepted state; Recovery Completion and
revoke-all clear runtime Session state. Frontend UI wiring remains pending and
must preserve the existing React/UI stack rather than reimplementing it.

Postgres is rechecked on every Bearer request. Redis cache loss or a stale
positive snapshot cannot change the authoritative Session result.

Phase 13.3 adds backend `AUTH_MODE=development|required`. Server deployments use
`required`, so clients must treat `401 UNAUTHENTICATED` as a login-required
state and should not rely on the development-user fallback outside local smoke
tests.

## 12. `importApi` Contract

Phase 8 import must be explicit and previewed. The frontend builds
`neo-chat-browser-import-v2.zip` from local IndexedDB/localforage plus OPFS bytes
and sends it only after user action. See
[`browser-data-import.md`](./browser-data-import.md) for the backend package
contract.

```ts
export interface ImportApi {
  buildBrowserImportPackage(
    input?: BuildBrowserImportPackageInput,
  ): Promise<BrowserImportPackage>;
  previewBrowserImport(
    input: BrowserImportPackage,
  ): Promise<ImportPreviewResponse>;
  commitBrowserImport(
    input: BrowserImportPackage,
  ): Promise<ImportCommitResponse>;
  getBrowserImportBatch(batchId: EntityId): Promise<ImportBatchStatus>;
  rollbackBrowserImportBatch(batchId: EntityId): Promise<void>;
}

export interface BuildBrowserImportPackageInput {
  includeConversations?: EntityId[];
  includeFiles?: boolean;
}

export interface BrowserImportPackage {
  file: File; // neo-chat-browser-import-v2.zip
  idempotencyKey: string;
  summary: {
    conversations: number;
    messages: number;
    files: number;
    bytes: number;
  };
}

export interface ImportPreviewResponse {
  summary: {
    conversations: number;
    messages: number;
    files: number;
    bytes: number;
    skippedDuplicates: number;
  };
  warnings: ImportIssue[];
  errors: ImportIssue[];
  commitAllowed: boolean;
}

export interface ImportCommitResponse {
  batchId: EntityId;
  status: "completed";
  created: {
    conversations: number;
    messages: number;
    files: number;
    attachments: number;
  };
  mappings: {
    conversations: Record<string, EntityId>;
    messages: Record<string, EntityId>;
    files: Record<string, EntityId>;
  };
  warnings: ImportIssue[];
}

export interface ImportBatchStatus {
  batchId: EntityId;
  status: "completed" | "rolled_back";
  createdAt: IsoDateTime;
}

export interface ImportIssue {
  code: string;
  path: string;
  message: string;
  severity: "warning" | "error";
}
```

Endpoint mapping:

| Method                       | Server Endpoint                      | Local Behavior                                             |
| ---------------------------- | ------------------------------------ | ---------------------------------------------------------- |
| `previewBrowserImport`       | `POST /v1/import/browser/preview`    | Validate generated package locally when server mode is off |
| `commitBrowserImport`        | `POST /v1/import/browser`            | No-op unless server mode is enabled                        |
| `getBrowserImportBatch`      | `GET /v1/import/browser/:batchId`    | Returns local preview/import state                         |
| `rollbackBrowserImportBatch` | `DELETE /v1/import/browser/:batchId` | Does not delete browser-local data                         |

Rules:

- Never call import APIs automatically on startup.
- Do not send raw `opfs://` URLs without packaged file bytes.
- Treat commit as all-or-nothing: failed imports return an error response and
  must not surface partial success in UI state.
- Do not include provider secrets, local secret envelopes, RAG tokens, plugin
  auth, cookies, or access tokens in the package.
- Existing all-data JSON export is not a valid server import package because it
  omits `session_messages_*` and OPFS bytes.

## 13. `settingsApi`, `providerApi`, and `pluginApi` Contracts

```ts
export interface SettingsApi {
  getRuntimeConfig(): Promise<RuntimeConfig>;
  getUserSettings(): Promise<UserSettingsSnapshot>;
  updateUserSettings(
    patch: Partial<UserSettingsSnapshot>,
  ): Promise<UserSettingsSnapshot>;
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

| Client Method                 | Server Endpoint             | Notes                                                                         |
| ----------------------------- | --------------------------- | ----------------------------------------------------------------------------- |
| `settings.getRuntimeConfig`   | `GET /v1/config`            | Runtime mode/capability bootstrap; `/api/config` remains local compatibility. |
| `settings.getUserSettings`    | `GET /v1/settings`          | User-visible settings only; no plaintext provider secrets.                    |
| `settings.updateUserSettings` | `PATCH /v1/settings`        | Partial update with server validation.                                        |
| `providers.listProviders`     | `GET /v1/providers`         | Returns provider metadata and `serverManaged` flags.                          |
| `providers.listModels`        | `POST /v1/providers/models` | Allows refresh and provider-specific lookup.                                  |
| `plugins.listAvailable`       | `GET /v1/plugins`           | Later phase; capability-gated.                                                |
| `plugins.listInstalled`       | `GET /v1/plugins/installed` | Later phase; capability-gated.                                                |
| `plugins.install`             | `POST /v1/plugins/install`  | Later phase; validate manifest before install.                                |
| `plugins.execute`             | `POST /v1/plugins/execute`  | Deferred until sandbox design exists.                                         |

The table above is the long-term server contract. Phase 11 must not call these
routes unless they are implemented by the Go router and explicitly reopened in
§20. The current Go backend does not register `/v1/config`, `/v1/settings`,
`/v1/providers*`, `/v1/auth*`, or `/v1/plugins*`.

Rules:

- Server mode sends provider IDs/model IDs, not plaintext API keys.
- Local mode can keep existing BYOK behavior.
- `RuntimeConfig.capabilities` gates UI visibility for features not yet migrated.
- `plugins` capability remains `false` for the first server MVP; `pluginApi` exists to avoid later component-level route coupling.

## 14. HTTP Client Rules

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
- `401` maps to `UNAUTHENTICATED` when the server returns an auth error
  envelope.
- `403` maps to `FORBIDDEN`.
- `404` maps to entity-specific `*_NOT_FOUND` where possible.
- `409` maps to `CONFLICT` or idempotency conflict.
- `413` maps to `FILE_TOO_LARGE`.
- `429` maps to `RATE_LIMITED` and should preserve retry metadata when
  available: `Retry-After`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and
  `X-RateLimit-Reset`.
- `5xx` maps to `SERVER_ERROR` and is recoverable unless explicitly marked otherwise.

## 15. Error Matrix

| Condition                      | Code                      |               Recoverable | Owner               |
| ------------------------------ | ------------------------- | ------------------------: | ------------------- |
| Missing/expired session        | `UNAUTHENTICATED`         |                       Yes | authApi/httpClient  |
| No access to conversation/file | `FORBIDDEN`               |                        No | backend             |
| Conversation missing           | `CONVERSATION_NOT_FOUND`  |                        No | chatApi             |
| Message missing                | `MESSAGE_NOT_FOUND`       |                        No | chatApi             |
| File too large                 | `FILE_TOO_LARGE`          |     Yes after user action | fileApi/backend     |
| Unsupported MIME               | `UNSUPPORTED_FILE_TYPE`   |     Yes after user action | fileApi/backend     |
| Provider not configured        | `PROVIDER_NOT_CONFIGURED` | Yes after settings change | providerApi/chatApi |
| Provider timeout               | `PROVIDER_TIMEOUT`        |                       Yes | provider adapter    |
| Stream interrupted             | `STREAM_INTERRUPTED`      |                       Yes | chatApi/httpClient  |
| Rate limited                   | `RATE_LIMITED`            |            Yes after wait | backend/Redis       |
| Invalid import/export payload  | `INVALID_IMPORT_PAYLOAD`  |    No for current payload | import phase later  |

## 16. Migration Sequence

| Step | Contract Work                                                                           | Runtime Change                         | Rollback                            |
| ---: | --------------------------------------------------------------------------------------- | -------------------------------------- | ----------------------------------- |
|    1 | Add client interfaces and factory                                                       | No behavior change                     | Remove unused interfaces            |
|    2 | Wrap current local chat service as `local.chatApi`                                      | Components still use local path        | Keep old imports                    |
|    3 | Add server HTTP adapter with health/version only; config waits for an implemented route | Can smoke test Go backend reachability | Switch `NEXT_PUBLIC_API_MODE=local` |
|    4 | Add server conversation/message CRUD                                                    | Server persistence opt-in              | Switch mode local                   |
|    5 | Add server stream events                                                                | SSE chat opt-in                        | Switch mode local                   |
|    6 | Add file API local adapter wrapper                                                      | No server files yet                    | Keep OPFS path                      |
|    7 | Add server file API                                                                     | MinIO opt-in                           | Disable `serverFiles` capability    |
|    8 | Add provider config/model APIs                                                          | Provider secrets server-side           | Fall back to local BYOK             |

## 17. Test Requirements

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

## 18. Acceptance Criteria for Phase 2

- `frontend-api-client.md` defines stable `chatApi`, `fileApi`, `authApi`, `settingsApi`, `providerApi`, and `importApi` contracts.
- Contract includes local/server mode rules.
- Contract includes SSE event envelope and event types.
- Contract includes error envelope and error matrix.
- Contract maps first server endpoints back to Phase 1 inventories.
- Contract includes provider/model identity, runtime config rollback, attachment source matrix, and plugin placeholder boundaries.
- `progress.md` and `process.md` are updated after review.

## 19. Open Decisions for Later Phases

- Whether `server` mode initially supports anonymous single-user access or mandatory login.
- Whether BYOK remains available in hosted server mode or only local mode.
- Whether server file downloads use backend streaming only or later presigned URLs.
- Whether search/RAG helpers stay in chat API or move to dedicated APIs after MVP.
- Whether plugin/tool execution is disabled, client-only, or sandboxed server-side in the first public server release.

## 20. Phase 11 Server-Mode Integration Implementation Contract

This section converts the broad frontend client contract into the concrete
Phase 11 implementation slice against the Go API that exists now. It is an
implementation contract, not a request to add frontend code in this document.

Authoritative inputs:

- Go routes registered by `mm-chat/backend/internal/httpserver/server.go`.
- Chat DTOs and validation in `mm-chat/backend/internal/chat/handler.go`.
- File DTOs and validation in `mm-chat/backend/internal/files/handler.go`.
- Existing legacy frontend shapes in `src/lib/chat/types.ts`.

### 20.1 Slice Objective

Wire the current app to server-backed chat without breaking local mode.

First implementation slice:

- Implement `server` mode for backend-supported chat CRUD and assistant SSE
  streaming only.
- Keep `local` mode behavior as-is behind the same `ChatApi` contract.
- Do not touch browser import UI, auth/login UI, RAG, plugin/tool execution,
  voice, document parsing, image generation, memory, or provider settings UI.
- Document file endpoint mapping now, but do not make file UI a dependency of
  the first chat CRUD + stream slice.

For this slice, "chat CRUD" means the currently implemented Go chat surface:

- create and list conversations;
- append and list messages;
- create/cancel assistant stream runs.

It does not include conversation `GET/PATCH/DELETE`, title generation,
message edit/delete, branching mutation, or generated-image/code/tool helpers
until matching Go endpoints exist.

### 20.2 Local/Server Adapter Boundary

Mode selection must happen once in the API client factory. Components, hooks,
and stores must not branch on raw route paths.

```text
React/UI call site
  -> NeoChatApiClient factory
  -> local adapter OR server adapter
  -> local stores/Next routes/OPFS OR Go HTTP/SSE API
```

Boundary rules:

- `local` adapter remains the compatibility owner for current browser state:
  Zustand, localforage/IndexedDB, `window.localStorage`, OPFS, and existing
  Next.js API routes.
- `server` adapter owns all Go calls under `NEXT_PUBLIC_API_BASE_URL`; it must
  not read or mutate browser-local chat persistence as source of truth.
- DTO-to-legacy mapping belongs inside the adapter/integration layer, not in
  presentation components.
- No component may directly call `/v1/chat/*`, `/v1/files/*`, `/api/chat`, or
  OPFS helpers as a server/local switch.
- Mixed mode is forbidden by default. A feature may remain local in server mode
  only when an explicit capability flag marks it out of scope for the current
  slice.
- `server` mode never sends provider API keys, local secret envelopes,
  `opfs://` URLs, MinIO/S3 object keys, buckets, or direct object-store URLs
  to the browser-visible contract.
- Browser server mode must choose one network edge before smoke testing:
  same-origin proxy/reverse proxy to Go, or direct
  `NEXT_PUBLIC_API_BASE_URL` only after backend CORS allowlisting is
  implemented and verified. The current Go API does not emit CORS headers, so
  a Next.js dev origin such as `http://localhost:3000` cannot directly fetch
  `http://127.0.0.1:8080` until that gap is closed.

Adapter ownership matrix:

| Concern                         | Local Adapter                            | Server Adapter                                   |
| ------------------------------- | ---------------------------------------- | ------------------------------------------------ |
| Conversation source of truth    | Current chat store/localforage           | Go `conversations` endpoints                     |
| Message source of truth         | Current `session_messages_*` trees       | Go `messages` endpoints                          |
| Generation                      | Existing `streamChatResponse` path       | `POST /v1/chat/.../stream` SSE                   |
| Files                           | OPFS/object URLs                         | Go `/v1/files`, backend-stream downloads         |
| Runtime mode                    | `NEXT_PUBLIC_API_MODE=local` or fallback | `NEXT_PUBLIC_API_MODE=server` plus base URL      |
| Unsupported features in slice 1 | Existing local behavior                  | Capability-gated off, no implicit local fallback |

Server mode must fail closed for unsupported server features. For example,
import/auth/RAG calls should stay unwired or capability-disabled rather than
silently uploading local data or switching one component back to a local route.

Unavailable Go routes in Phase 11:

- Do not call `/v1/config`, `/v1/settings`, `/v1/providers*`, `/v1/auth*`, or
  `/v1/plugins*` from server mode in this phase.
- Mode, base URL, and coarse capability flags come from build-time env,
  same-origin local config, or a static adapter capability object until a Go
  config endpoint is implemented.
- Missing server features return an explicit unsupported/capability-disabled
  result. They must not silently fall back to browser-local persistence in
  server mode.

### 20.3 Server Adapter Endpoint Mapping

The server adapter must target only implemented Go routes in Phase 11 slice 1.
Earlier future mappings in §8.2 remain roadmap items.

#### Conversations

| Client method        | Go endpoint                   | Phase 11 behavior                                                                   |
| -------------------- | ----------------------------- | ----------------------------------------------------------------------------------- |
| `createConversation` | `POST /v1/chat/conversations` | Send `title`, `modelRef`, optional `systemInstruction`, `config`, `idempotencyKey`. |
| `listConversations`  | `GET /v1/chat/conversations`  | Parse `{ items }`; no cursor support yet.                                           |
| `getConversation`    | none                          | Derive from `listConversations` only if needed; otherwise report unsupported.       |
| `updateConversation` | none                          | Not implemented in server mode slice 1.                                             |
| `deleteConversation` | none                          | Not implemented in server mode slice 1.                                             |
| `generateTitle`      | none                          | Keep out of server mode slice 1.                                                    |

Server response shape is `ConversationDTO`:

```ts
{
  id: string;
  title: string;
  status: "active" | "archived" | "deleted";
  modelRef?: ModelRef;
  messageCount: number;
  config: Record<string, unknown>;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}
```

#### Messages

| Client method                  | Go endpoint                                 | Phase 11 behavior                          |
| ------------------------------ | ------------------------------------------- | ------------------------------------------ |
| `appendUserMessage`            | `POST /v1/chat/conversations/{id}/messages` | Creates one completed `role=user` message. |
| `listMessages`                 | `GET /v1/chat/conversations/{id}/messages`  | Parse `{ items }` in sequence order.       |
| message edit/delete/regenerate | none                                        | Not implemented in server mode slice 1.    |

`appendUserMessage` must send only fields accepted by the Go handler:

```ts
{
  role?: "user";
  content: string;
  attachments?: Array<{ source?: "server"; fileId: string; purpose?: string }>;
  parentMessageId?: string;
  metadata?: Record<string, unknown>;
  idempotencyKey?: string;
}
```

Rules:

- The adapter may omit `role`; if present it must be `"user"`.
- `content` must be non-blank before sending.
- Server-managed fields (`id`, `conversationId`, `status`, `modelRef`,
  `sequenceNo`, timestamps, output blocks, identity hints) must not be sent.
- Attachments are server file references only. `opfs`, `inline`, `remote`,
  base64 data, object keys, and bucket names are forbidden here.

#### Stream Runs

| Client method            | Go endpoint                               | Phase 11 behavior                                           |
| ------------------------ | ----------------------------------------- | ----------------------------------------------------------- |
| `streamAssistantMessage` | `POST /v1/chat/conversations/{id}/stream` | POST body, parse `text/event-stream`.                       |
| `cancelRun`              | `POST /v1/chat/runs/{runId}/cancel`       | Durable cancel; also interrupts active same-process stream. |

`streamAssistantMessage` body:

```ts
{
  userMessageId: string;
  modelRef: ModelRef;
  config?: Record<string, unknown>;
  systemInstruction?: string;
  systemPrompt?: string;
  metadata?: Record<string, unknown>;
  idempotencyKey: string;
}
```

Rules:

- The caller must persist the user message first via `appendUserMessage`.
- The stream body must not include `content`, `attachments`, `role`, `status`,
  timestamps, identity hints, or other server-managed message fields.
- `idempotencyKey` is required. The adapter may generate one before the
  request, but must not reuse it for a different assistant run.
- `modelRef.providerId` must match the configured Go provider. Current aliases
  are enforced by the backend provider resolver.

#### Files

The file mapping is part of the Phase 11 contract but is not required for the
first chat CRUD + stream slice.

| Client method        | Go endpoint                      | Phase 11 mapping                                            |
| -------------------- | -------------------------------- | ----------------------------------------------------------- |
| `files.upload`       | `POST /v1/files`                 | `multipart/form-data` with `file`, `purpose`, optional IDs. |
| `files.getMetadata`  | `GET /v1/files/{fileId}`         | Returns metadata plus relative `downloadUrl`.               |
| `files.getContent`   | `GET /v1/files/{fileId}/content` | Optional `?disposition=attachment`.                         |
| `files.delete`       | `DELETE /v1/files/{fileId}`      | `204 No Content`; no JSON body.                             |
| `files.getObjectUrl` | composed from `getContent`       | Browser object URL/cache wrapper around backend stream.     |

Rules:

- There is no file list endpoint in the current Go API.
- `downloadUrl` is a backend-stream path, not a presigned object-store URL.
- The adapter must prefix relative `downloadUrl` values with the configured
  server base URL when making browser fetches.
- Upload purpose must be one of `chat`, `workspace`, `knowledge`, `image`,
  `audio`, or `export`.
- Server file metadata must never expose storage backend, object key, bucket,
  local path, or MinIO/S3 URL.

### 20.4 SSE Event Handling

Server-mode streaming uses `fetch()` with `POST` and a request body. Do not use
`EventSource` for this endpoint because native `EventSource` cannot send the
required JSON body.

Wire rules:

- Request headers include `Accept: text/event-stream` and
  `Content-Type: application/json`.
- The parser reads `ReadableStream` bytes with `TextDecoder`, preserves partial
  frames across chunks, and splits events on blank lines.
- Each server frame has one named `event:` line and one JSON `data:` object.
  The adapter should tolerate multi-line `data:` per SSE rules even though the
  current Go server emits one line.
- `event:` must match `data.type`; mismatches are recoverable
  `STREAM_PROTOCOL_ERROR` events.
- Ignore comments/empty keepalive frames if they are added later.

Current Go event types:

| Event               | Handler       | State effect                                                          |
| ------------------- | ------------- | --------------------------------------------------------------------- |
| `message.started`   | `onStarted`   | Capture `runId` and assistant `messageId`; mark generation streaming. |
| `message.delta`     | `onDelta`     | Append `delta` to transient assistant content.                        |
| `usage.updated`     | `onUsage`     | Merge optional usage metadata.                                        |
| `message.completed` | `onCompleted` | Replace transient content with persisted `message`; terminal success. |
| `message.error`     | `onError`     | Terminal failure after SSE start.                                     |
| `message.cancelled` | `onCancelled` | Terminal cancellation.                                                |

Future events already defined in §9 (`message.reasoning_delta`, `tool.*`,
`search.updated`, `image.generated`, `timing.updated`) must remain parsed by
type but capability-gated. In Phase 11 slice 1, receiving those events should
not trigger plugin/RAG/import UI.

Sequence handling:

- `sequence` is monotonic per `runId`.
- Duplicate sequence numbers are ignored.
- A sequence gap is recoverable: emit or synthesize `STREAM_INTERRUPTED`,
  stop applying more deltas for that run, and refresh via `listMessages` when
  the request settles.
- `message.completed.message` is authoritative over accumulated deltas.
- Exactly one terminal event is allowed. Extra frames after a terminal event
  are ignored.

Abort/cancel handling:

- If `AbortSignal` fires before `message.started`, abort the fetch only; no
  `runId` exists yet.
- If `AbortSignal` fires after `message.started`, call
  `POST /v1/chat/runs/{runId}/cancel`, then treat `message.cancelled` or the
  cancel JSON response as terminal.
- A cancelled or failed stream must not be rendered as completed even if some
  deltas were received.

### 20.5 Error Handling Contract

All non-stream JSON errors use the standard envelope:

```json
{
  "error": {
    "code": "CONVERSATION_NOT_FOUND",
    "message": "conversation not found"
  }
}
```

Server adapter normalization:

- Parse the envelope for every non-`2xx` JSON response.
- Preserve backend `error.code` and `error.message` exactly.
- Add a synthesized `requestId` only when a response header supplies one or the
  adapter creates a local correlation ID; do not invent backend trace data.
- `204 No Content` from file delete is success and must not be JSON-parsed.
- Network/CORS/timeout failures map to `NETWORK_ERROR` or
  `STREAM_INTERRUPTED` with `recoverable: true`.
- Invalid JSON from a JSON endpoint maps to `INVALID_SERVER_RESPONSE`.
- Invalid SSE framing maps to `STREAM_PROTOCOL_ERROR`.

Pre-SSE errors from `POST /stream` are ordinary JSON errors, including:

- `400 MODEL_REF_REQUIRED`, `IDEMPOTENCY_KEY_REQUIRED`,
  `INVALID_USER_MESSAGE_ID`, `INVALID_JSON`, `INVALID_CONVERSATION_ID`,
  `VALIDATION_ERROR`, `FORBIDDEN_MESSAGE_FIELD`, `UNSUPPORTED_PROVIDER`;
- `404 CONVERSATION_NOT_FOUND`;
- `409 IDEMPOTENCY_CONFLICT`;
- `500 STREAMING_UNSUPPORTED`;
- `502 PROVIDER_ERROR`;
- `503 DATABASE_REQUIRED` or `PROVIDER_REQUIRED`.

Cancel errors from `POST /v1/chat/runs/{runId}/cancel` include
`INVALID_RUN_ID`, `RUN_NOT_FOUND`, and `RUN_NOT_CANCELLABLE`. This list is a
Phase 11 implementation aid, not a closed enum; preserve backend error codes
from [`chat-stream-api.md`](./chat-stream-api.md) and the Go handlers.

After SSE starts, provider/finalization failures arrive as `message.error`
frames and must produce `ChatRunResult.status = "failed"`. The adapter must not
retry automatically with the same idempotency key; retries require a new user
action and a new assistant-run key.

Rate-limit and infrastructure handling:

- `429 RATE_LIMITED` remains recoverable after waiting. Preserve
  `Retry-After`, `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and
  `X-RateLimit-Reset` when present.
- `503 DATABASE_REQUIRED`, `STORAGE_REQUIRED`, and `PROVIDER_REQUIRED` are
  configuration errors. Surface them in server-mode smoke feedback and keep
  rollback to local mode available.
- `405 METHOD_NOT_ALLOWED` should include the backend `Allow` header when
  surfaced for debugging.

### 20.6 ISO Date to Legacy Timestamp Compatibility

The Go API emits UTC RFC3339/RFC3339Nano strings. The current frontend still
uses millisecond timestamps in several core types:

- `Session.updatedAt: number`;
- `Message.timestamp: number`;
- `MessageVersion.timestamp: number`;
- timing fields such as `startTime`, `endTime`, and `duration`.

Compatibility rules:

- Convert ISO strings to Unix epoch milliseconds with `Date.parse`.
- Invalid or empty ISO values are adapter errors for required fields. Use a
  documented fallback only for optional display fields.
- Conversation mapping:
  - `ConversationDTO.updatedAt` -> legacy `Session.updatedAt`;
  - `ConversationDTO.modelRef` -> legacy `Session.model` using the existing
    provider/model string convention;
  - `ConversationDTO.config` -> legacy `Session.config`.
- Message mapping:
  - API `role: "assistant"` -> legacy `role: "model"`;
  - API `role: "user"` -> legacy `role: "user"`;
  - API `createdAt` -> legacy `Message.timestamp`;
  - API `completedAt`, when present, may feed legacy timing `endTime`;
  - API `modelRef` -> legacy `Message.model`;
  - API `usage`/`usage.updated` -> legacy usage fields without dropping the
    provider-neutral payload.
- Server `sequenceNo` is ordering authority for server-mode message lists.
  Legacy timestamps are for display/sorting compatibility only.
- Local adapter may keep timestamps as numbers internally, but any value that
  crosses the shared DTO boundary must still satisfy the `IsoDateTime` contract.

Date conversion is part of the adapter boundary. Components should continue to
receive the legacy shape they already render until a separate UI refactor
changes those types.

### 20.7 Rollback Contract

Primary rollback is a configuration switch:

```env
NEXT_PUBLIC_API_MODE=local
```

Rollback rules:

- Missing, invalid, or unsupported mode resolves to `local`.
- If `NEXT_PUBLIC_API_MODE` is baked into a built Next.js bundle, rollback
  requires rebuild/redeploy or a runtime config layer that explicitly overrides
  it.
- Rollback does not delete Postgres, Redis, or object-store data.
- Server-mode data created before rollback remains on the Go backend for later
  debugging or re-enable.
- Browser-local data remains untouched by server-mode slice 1 because import UI
  is out of scope.
- If a release is partially deployed, prefer `local` mode over mixed component
  fallbacks. One factory switch must determine behavior.

Operational rollback smoke:

1. Set `NEXT_PUBLIC_API_MODE=local`.
2. Restart or rebuild the frontend as required by the deployment target.
3. Load the app with the Go backend stopped.
4. Confirm local chat creation, local streaming, and local history still work.

### 20.8 Verification Checklist

#### Backend ready

- `mm-chat` Go backend is running with Postgres migrations applied.
- `/health` returns `200 {"status":"healthy"}`.
- `/ready` returns `200 {"status":"ready"}`. Server deployments may include
  additive `checks` detail, for example `database`, `redis`, and `storage`;
  every returned check should have `{"status":"ready"}` before browser smoke.
- `/v1/version` returns `200` with a version string.
- `GET /v1/chat/conversations` returns `200` and an `{ items: [] }` page or
  existing conversations.
- `POST /v1/chat/conversations` creates a conversation.
- `POST /v1/chat/conversations/{id}/messages` appends a completed user
  message.
- `POST /v1/chat/conversations/{id}/stream` emits at least
  `message.started`, zero or more `message.delta`, and one terminal event.
- `POST /v1/chat/runs/{runId}/cancel` cancels a streaming run when exercised.
- File endpoints may be smoke-tested separately with `POST /v1/files`,
  `GET /v1/files/{id}`, `GET /v1/files/{id}/content`, and
  `DELETE /v1/files/{id}`, but they are not required for slice 1 UI wiring.

#### Browser smoke in server mode

- Set `NEXT_PUBLIC_API_MODE=server` and
  `NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8080` or the deployed backend URL.
- If the frontend origin differs from the backend origin, verify one of these
  first: same-origin proxy routing to Go, or explicit backend CORS headers for
  the frontend origin. Do not use wildcard CORS for credentialed or hosted
  deployments.
- Create a new chat from the existing UI without directly calling Go routes
  from components.
- Send one user message; verify the network sequence is:
  1. `POST /v1/chat/conversations` when needed;
  2. `POST /v1/chat/conversations/{id}/messages`;
  3. `POST /v1/chat/conversations/{id}/stream`.
- Verify streamed deltas render once and the final persisted assistant message
  replaces transient content.
- Refresh the browser and confirm `listConversations` + `listMessages` restore
  server data.
- Cancel one active stream and confirm UI terminal state is cancelled, not
  failed or completed.
- Confirm no Phase 11 slice-1 path calls import, auth, RAG, plugin, voice,
  document, image, or code-execution routes.
- Confirm ISO dates render as valid legacy timestamps and conversation ordering
  is stable after refresh.

#### Local mode regression

- Set `NEXT_PUBLIC_API_MODE=local`.
- Run the app with the Go backend unavailable.
- Create a local conversation, send a local streamed message, refresh, and
  verify existing local history remains available.
- Verify OPFS/local attachments still render through the local path.
- Verify existing local helpers not in server slice 1 still behave as before:
  title helpers, RAG/search settings, plugins where enabled, voice, and
  document parsing.
- Confirm no server-mode adapter code is imported in a way that performs
  network calls during local bootstrap.

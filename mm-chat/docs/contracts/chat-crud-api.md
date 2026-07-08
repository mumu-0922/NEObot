# Phase 5.1/6.3 Chat CRUD API Contract

## 1. Purpose

This contract defines the first durable chat path for the Go backend: create and
list conversations, then append and list completed user messages from Postgres.
It proves the HTTP handler, service validation, and repository boundary before
auth, Redis, MinIO, RAG, or browser-data import are added. Phase 6.3 extends
the user-message path with server-uploaded file attachment links.

```text
HTTP client -> /v1/chat/conversations* -> Go service -> Postgres
```

## 2. Endpoints

| Method | Path | Success | Purpose |
| --- | --- | --- | --- |
| `POST` | `/v1/chat/conversations` | `201 Created` | Create one conversation. |
| `GET` | `/v1/chat/conversations` | `200 OK` | List active conversations. |
| `POST` | `/v1/chat/conversations/{id}/messages` | `201 Created` | Append one completed user message. |
| `GET` | `/v1/chat/conversations/{id}/messages` | `200 OK` | List messages in sequence order. |

All responses are JSON with `Content-Type: application/json; charset=utf-8` and
`X-Content-Type-Options: nosniff`.

## 3. Fixed Development User

Phase 5.1 has no auth. Every request is scoped to the backend-owned development
user:

```text
id:          00000000-0000-0000-0000-000000000001
displayName: Development User
```

Requests must not accept `userId`, `ownerId`, sessions, bearer tokens, or
impersonation parameters. Repository reads and writes must always filter by this
user ID before returning conversations or messages.

## 4. Shared DTO Rules

```ts
export type EntityId = string; // UUID
export type IsoDateTime = string; // UTC RFC3339
export type JsonObject = Record<string, unknown>;
export type JsonArray = unknown[];

export interface ApiPage<T> {
  items: T[];
  nextCursor?: string;
}
```

Phase 5.1 reserves the `ApiPage` shape but does not implement cursor
pagination; list endpoints currently return the full active set for the fixed
user and omit `nextCursor`.

## 5. Conversations

```ts
export interface ModelRef {
  providerId: string;
  modelId: string;
  displayName?: string;
}

export interface ConversationDto {
  id: EntityId;
  title: string;
  status: "active" | "archived" | "deleted";
  modelRef?: ModelRef;
  messageCount: number;
  config: JsonObject;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}

export interface CreateConversationRequest {
  title?: string;
  modelRef?: ModelRef;
  systemInstruction?: string;
  systemPrompt?: string; // compatibility alias
  config?: JsonObject;
  metadata?: JsonObject; // compatibility alias when config is absent
  idempotencyKey?: string;
}
```

Rules:

- Missing `title` stores `""`; whitespace is trimmed.
- `modelRef.providerId` and `modelRef.modelId` map to Postgres
  `model_provider` and `model_id`.
- `systemInstruction` is preferred; `systemPrompt` is accepted as a temporary
  alias.
- `config` is the public JSON object; `metadata` is accepted only as a fallback
  alias.
- Server-managed or caller-identity fields such as `id`, `userId`, `ownerId`,
  `sessionId`, `session`, `bearerToken`, `accessToken`, `authorization`,
  `impersonateUserId`, `status`, `messageCount`, `createdAt`, `updatedAt`, and
  `deletedAt` are rejected with `400 VALIDATION_ERROR`.
- Legacy top-level `modelProvider` and `modelId` are rejected; callers must use
  `modelRef`.
- Duplicate non-empty `idempotencyKey` values return `409 IDEMPOTENCY_CONFLICT`.
  Phase 5.1 does not replay the original response.

## 6. Messages

```ts
export interface ChatMessageDto {
  id: EntityId;
  conversationId: EntityId;
  sequenceNo: number;
  role: "system" | "user" | "assistant" | "tool";
  status: "pending" | "streaming" | "completed" | "failed" | "cancelled";
  content: string;
  modelRef?: ModelRef;
  attachments: ServerAttachmentDto[];
  outputBlocks: JsonArray;
  metadata: JsonObject;
  parentMessageId?: EntityId;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
  completedAt?: IsoDateTime;
}

export interface ServerAttachmentDto {
  id: EntityId; // message_attachments.id
  source: "server";
  fileId: EntityId;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: "input" | "image" | "knowledge_source";
}

export interface AppendMessageAttachmentInput {
  source?: "server"; // omitted defaults to server
  fileId: EntityId;
  purpose?: "input" | "image" | "knowledge_source" | "chat" | "knowledge";
}

export interface AppendUserMessageRequest {
  role?: "user"; // omitted defaults to user; any other role is rejected
  content: string;
  attachments?: AppendMessageAttachmentInput[];
  parentMessageId?: EntityId;
  metadata?: JsonObject;
  idempotencyKey?: string;
}
```

Rules:

- This endpoint creates user messages only: `role = "user"`,
  `status = "completed"`, and `completedAt` is set on insert.
- `role: "assistant"`, `"tool"`, or `"system"` returns
  `400 FORBIDDEN_MESSAGE_FIELD`; provider-created assistant/tool rows are later
  streaming work.
- Server-managed fields such as `id`, `conversationId`, `userId`, `sequenceNo`,
  `status`, `modelRef`, `modelProvider`, `modelId`, `providerMessageId`,
  `outputBlocks`, `errorCode`, `errorMessage`, `createdAt`, `updatedAt`,
  `completedAt`, and `deletedAt` return `400 FORBIDDEN_MESSAGE_FIELD`.
- Caller-identity hints such as `ownerId`, `sessionId`, `session`,
  `bearerToken`, `accessToken`, `authorization`, and `impersonateUserId` also
  return `400 FORBIDDEN_MESSAGE_FIELD`.
- `content` must be non-blank after trimming.
- `attachments`, when present, must reference already uploaded server files.
  `source` may be omitted or `"server"`; `opfs`, `inline`, and `remote`
  attachment sources are rejected on the Go API boundary.
- Attachment `fileId` values must be UUIDs, unique per message, owned by the
  fixed development user, not deleted, and `upload_status = "available"`.
- Attachment purpose defaults to `input`. The API accepts `input`, `image`,
  `knowledge_source`, plus compatibility aliases `chat -> input` and
  `knowledge -> knowledge_source`.
- Message responses return attachment metadata and backend file IDs only. They
  must not expose object keys, local paths, buckets, or MinIO/S3 URLs.
- `parentMessageId`, when present, must be a UUID string.
- `sequenceNo` is repository-owned and assigned inside the conversation lock.
- Duplicate non-empty `idempotencyKey` values return `409 IDEMPOTENCY_CONFLICT`.

## 7. Error Contract

```json
{
  "error": {
    "code": "EMPTY_CONTENT",
    "message": "message content is required"
  }
}
```

| HTTP | Code | When |
| --- | --- | --- |
| `400` | `INVALID_JSON` | Body is malformed JSON or contains multiple JSON values. |
| `400` | `INVALID_CONVERSATION_ID` | Path conversation ID is not a UUID. |
| `400` | `INVALID_PARENT_MESSAGE_ID` | `parentMessageId` is not a UUID. |
| `400` | `INVALID_ATTACHMENT_FILE_ID` | Attachment `fileId` is missing or not a UUID. |
| `400` | `INVALID_ATTACHMENT_PURPOSE` | Attachment purpose is unsupported. |
| `400` | `UNSUPPORTED_ATTACHMENT_SOURCE` | Attachment source is not `server`. |
| `400` | `DUPLICATE_ATTACHMENT` | The same file is attached more than once to one message. |
| `400` | `TOO_MANY_ATTACHMENTS` | More than 20 attachments are supplied for one message. |
| `400` | `VALIDATION_ERROR` | Conversation request tries to write server-managed fields. |
| `400` | `EMPTY_CONTENT` | Message content is blank. |
| `400` | `FORBIDDEN_MESSAGE_FIELD` | Client tries to create a non-user message. |
| `404` | `CONVERSATION_NOT_FOUND` | Conversation is missing, deleted, or not owned by the fixed user. |
| `404` | `FILE_NOT_FOUND` | Attachment file is absent, deleted, unavailable, or not owned by the fixed user. |
| `405` | `METHOD_NOT_ALLOWED` | Method is not allowed; response includes `Allow`. |
| `409` | `IDEMPOTENCY_CONFLICT` | Non-empty idempotency key already exists in scope. |
| `429` | `RATE_LIMITED` | Redis rate-limit middleware blocked the request before handler execution. |
| `503` | `DATABASE_REQUIRED` | `DATABASE_URL` is empty and chat persistence is disabled. |
| `500` | `INTERNAL_ERROR` | Unexpected server error after sensitive details are scrubbed. |

`/ready` still reports `DATABASE_NOT_READY` when a configured database later
fails ping; chat request handlers do not expose raw SQL or connection details.

When DB wiring is disabled, `DATABASE_REQUIRED` takes precedence over request
body validation for all four chat endpoints, including malformed POST bodies.

## 8. Repository Boundary

Allowed tables in Phase 5.1/6.3:

```text
users
conversations
messages
files
message_attachments
```

Allowed behavior:

- Lazily ensure the fixed development user.
- Insert/list active conversations owned by that user.
- Insert/list messages only after proving the parent conversation is owned by
  that user.
- In Phase 6.3, link uploaded files to newly created user messages inside the
  same transaction by inserting `message_attachments`.
- Attachment writes must verify file ownership and availability through
  `files.user_id`, `deleted_at IS NULL`, and `upload_status = 'available'`
  before inserting a link.
- Store `idempotency_key` as a retry guard without replay semantics.
  Duplicate detection is limited to the conversation/message idempotency unique
  indexes, not arbitrary database uniqueness failures.

Not allowed yet: `sessions`, `provider_configs`, `audit_logs`, Redis,
MinIO/S3, RAG, auth, browser IndexedDB/OPFS import, or sending attachments to
the model provider as multimodal input. SSE streaming still consumes a
persisted `userMessageId` and rejects `attachments` in the stream request body.

## 9. Verification Targets

- With `DATABASE_URL` empty, all chat endpoints return `503 DATABASE_REQUIRED`.
- With migrated Postgres, conversation create/list round-trips through
  `users`, `conversations`, and `messages` only.
- Unknown or foreign conversation IDs return `404 CONVERSATION_NOT_FOUND`.
- Non-user message roles return `400 FORBIDDEN_MESSAGE_FIELD` and are not
  persisted.
- Server file attachments round-trip through `message_attachments` and are
  returned by both create and list message endpoints.
- Missing, deleted, foreign, duplicated, or non-server attachments are rejected
  without creating the message.
- Duplicate idempotency keys return `409 IDEMPOTENCY_CONFLICT` instead of a raw
  database error.

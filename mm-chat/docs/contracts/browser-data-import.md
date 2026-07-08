# Phase 8 Browser Data Import Contract

## 1. Purpose

Phase 8 imports explicit, user-selected browser-local Neo Chat data into the
server-backed stack. The goal is safe migration from IndexedDB/localforage and
OPFS into Postgres plus MinIO/S3 without silently uploading local data.

```text
browser local stores + OPFS
  -> neo-chat-browser-import-v2.zip
  -> preview validation
  -> user confirmation
  -> Postgres conversations/messages/files + object storage bytes
```

## 2. Non-Goals

- No automatic background upload of IndexedDB, localStorage, OPFS, memories, or
  provider settings.
- No import of plaintext provider keys, RAG tokens, plugin credentials, access
  tokens, cookies, or local secret envelopes.
- No backend execution of imported tool calls, plugins, code, or HTML.
- No direct MinIO/S3 URL exposure to the browser.
- No assumption that local IDs are server IDs.

## 3. Import Package

Phase 8 uses a new ZIP package. The existing `neo-chat-export-YYYY-MM-DD.json`
(`AppExportPayload` v1) is source material only; it omits `session_messages_*`
and OPFS bytes, so the backend must reject it as a direct server import input.

Package name convention:

```text
neo-chat-browser-import-v2.zip
```

Required uploaded ZIP layout:

```text
neo-chat-browser-import-v2.zip
├── manifest.json
└── files/
    └── sha256/
        └── {sha256}        # binary blobs referenced by manifest file blobPath values
```

Diagnostic exports such as `stores/*.json` or
`messages/{sessionClientId}.json` may be generated locally for debugging, but
they are not valid entries in the uploaded server import ZIP and must not be
submitted to the backend.

Rules:

- The backend accepts only `manifest.json` and `files/sha256/*`. It must reject
  every non-whitelisted ZIP entry, including `stores/*`, `messages/*`,
  scripts, HTML, hidden files, and path aliases.
- ZIP entries must be relative, normalized UTF-8 paths; absolute paths, `..`,
  empty segments, backslashes, symlinks, duplicate paths, and encrypted ZIPs are
  rejected.
- `files/sha256/{hash}` names must match lowercase 64-char SHA-256 hex values.
- Every OPFS or inline attachment imported as a server file must have a matching
  file entry in the ZIP. Remote URL attachments are metadata-only in Phase 8 and
  are not fetched by the backend.
- The browser exporter must enumerate `appDb.keys()` and include all
  `session_messages_{sessionId}` records; the current five-key all-data export is
  insufficient.

`POST /v1/import/browser/preview` and `POST /v1/import/browser` both accept
`multipart/form-data` with one ZIP part:

| Part | Required | Notes |
| --- | ---: | --- |
| `package` | yes | `application/zip` or `application/octet-stream`; contains `manifest.json` and file blobs. |

```ts
export interface BrowserImportManifestV2 {
  format: "neo-chat-browser-import";
  schemaVersion: "mm-chat.browser-import.v2";
  storageVersion: number;
  appExportVersion?: number;
  exportedAt?: IsoDateTime;
  generatedAt: IsoDateTime;
  idempotencyKey: string;
  source: {
    app: "neo-chat";
    origin?: string;
  };
  counts: {
    conversations: number;
    messages: number;
    files: number;
    bytes: number;
  };
  opfs: {
    referencedUrls: string[];
    missingUrls: string[];
    orphanUrls: string[];
  };
  options?: {
    onDuplicate?: "skip" | "copy";
    allowMissingFiles?: false;
  };
  conversations: ImportConversation[];
  messages: ImportMessage[];
  files: ImportFile[];
  workspaces?: ImportWorkspace[];
  deferred?: {
    knowledgeCollections?: number;
    memories?: number;
    providerSettings?: number;
  };
}
```

## 4. Normalized DTOs

```ts
export interface ImportConversation {
  clientId: string;
  title: string;
  status?: "active" | "archived";
  modelRef?: { providerId: string; modelId: string };
  systemInstruction?: string;
  workspaceClientId?: string;
  pinned?: boolean;
  config?: Record<string, unknown>;
  createdAt?: IsoDateTime;
  updatedAt: IsoDateTime;
}

export interface ImportMessage {
  clientId: string;
  conversationClientId: string;
  parentClientId?: string;
  sequenceNo: number;
  role: "system" | "user" | "assistant" | "tool";
  status?: "completed" | "failed" | "cancelled";
  content: string;
  modelRef?: { providerId: string; modelId: string };
  attachments?: ImportAttachment[];
  outputBlocks?: unknown[];
  metadata?: Record<string, unknown>;
  createdAt: IsoDateTime;
  completedAt?: IsoDateTime;
}

export interface ImportAttachment {
  clientAttachmentId: string;
  source: "file" | "remote" | "knowledge_ref";
  clientFileId?: string;
  fileName: string;
  mimeType: string;
  size?: number;
  sha256?: string;
  url?: string;
  purpose?: "input" | "image" | "knowledge_source";
}

export interface ImportFile {
  clientFileId: string;
  source: "opfs" | "inline";
  originalUrl?: `opfs://${string}`;
  sourceAttachmentIds: string[];
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  blobPath: `files/sha256/${string}`;
  purpose: "chat" | "workspace" | "knowledge" | "image" | "audio" | "export";
}

export interface ImportWorkspace {
  clientId: string;
  name: string;
  systemPrompt?: string;
  color?: string;
}
```

Role mapping rule: existing frontend `Message.role = "model"` is normalized to
`assistant` before upload.

Timestamp rule: existing millisecond timestamps are converted to UTC RFC3339 in
the browser. The backend rejects invalid, future-skewed, or non-UTC timestamps
instead of guessing.

## 5. Endpoints

```http
POST   /v1/import/browser/preview
POST   /v1/import/browser
GET    /v1/import/browser/{batchId}
DELETE /v1/import/browser/{batchId}
```

### `POST /v1/import/browser/preview`

Validates the ZIP package, manifest, and referenced blobs without writing
conversations, messages, or files. The browser keeps the selected files locally
and submits the same package again for commit after user confirmation.

```ts
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
```

### `POST /v1/import/browser`

Commits the exact package after confirmation. The server creates a batch ID,
then inserts conversations/messages and uploads file bytes.

Runtime rollout note: the first Go runtime slice commits conversations and
messages only. Packages containing `files[]` or file-backed attachments must be
rejected instead of imported without attachments until the MinIO/S3 attachment
slice is implemented.

```ts
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
```

### `DELETE /v1/import/browser/{batchId}`

Rolls back an imported batch when it has not been modified after import. Rollback
soft-deletes imported conversations/messages/files and deletes object-store
bytes for files created by the batch. If later user edits are detected, return
`409 IMPORT_BATCH_MODIFIED` and require an explicit force-delete design in a
later phase.

### `GET /v1/import/browser/{batchId}`

Returns committed batch state for UI refresh and rollback confirmation.

```ts
export interface ImportBatchStatus {
  batchId: EntityId;
  status: "completed" | "rolled_back";
  createdAt: IsoDateTime;
}
```

If the batch does not exist for the current user, return
`404 IMPORT_BATCH_NOT_FOUND`.

## 6. Validation Rules

Preview and commit use identical validation.

- `schemaVersion` must equal `mm-chat.browser-import.v2` and `format` must equal `neo-chat-browser-import`.
- `idempotencyKey` is required and scoped to the current user.
- `clientId` values must be unique within their type and must not be trusted as
  server IDs.
- Every message must reference an existing `conversationClientId`.
- `parentClientId`, when present, must reference a message in the same
  conversation and must not create a cycle.
- `sequenceNo` must be unique and gap-free per conversation after sorting.
- `content` may be empty only for messages with at least one attachment or a
  non-empty `outputBlocks` array.
- File attachments with `source = "file"` must reference an `ImportFile` and a
  matching ZIP blob at `blobPath`.
- File size and SHA-256 must match the uploaded ZIP blob bytes.
- Per-file size must respect `MAX_UPLOAD_BYTES`; total import size must be
  governed by a future `IMPORT_MAX_BYTES` setting.
- `remote` attachments are metadata-only in Phase 8 and must not trigger backend
  fetches.
- Inline `data` attachments must be decoded by the browser exporter into
  `ImportFile` blobs before upload; raw base64 data is forbidden in the server
  manifest.
- Provider secrets, plugin auth, RAG tokens, access tokens, cookie material, and
  local encrypted secret envelopes are rejected if present in importable fields.

## 7. Persistence Mapping

| Manifest item | Server target |
| --- | --- |
| `ImportConversation` | `conversations` row scoped to current/fixed user. |
| `ImportMessage` | `messages` row with preserved order, role, status, content, metadata. |
| `ImportFile` + ZIP blob | object storage write + `files` metadata row. |
| `ImportAttachment` | `message_attachments` row linking imported message/file. |
| `ImportWorkspace` | Conversation metadata only until server workspaces exist. |

Each imported row carries `metadata.import.batchId` and original client IDs for
rollback and audit. Runtime migration `003_import_batches` stores batch
idempotency, response replay, and `completed|rolled_back` state; row metadata
remains queryable so rollback can find imported conversations/messages.

## 8. Error Contract

```ts
export interface ImportIssue {
  code:
    | "INVALID_IMPORT_PAYLOAD"
    | "UNSUPPORTED_SCHEMA_VERSION"
    | "INVALID_IMPORT_PACKAGE"
    | "MISSING_FILE_BLOB"
    | "FILE_HASH_MISMATCH"
    | "FILE_TOO_LARGE"
    | "DUPLICATE_CLIENT_ID"
    | "INVALID_MESSAGE_TREE"
    | "IMPORT_BATCH_MODIFIED"
    | "FORBIDDEN_SECRET_FIELD";
  path: string;
  message: string;
  severity: "warning" | "error";
}
```

HTTP mapping:

| HTTP | Code | When |
| --- | --- | --- |
| `400` | `INVALID_IMPORT_PAYLOAD` | JSON/multipart/schema validation fails. |
| `400` | `UNSUPPORTED_SCHEMA_VERSION` | Unknown `schemaVersion`. |
| `400` | `INVALID_IMPORT_PACKAGE` | ZIP layout or path validation fails. |
| `400` | `MISSING_FILE_BLOB` | Referenced ZIP blob is absent. |
| `400` | `FILE_HASH_MISMATCH` | Uploaded bytes do not match declared SHA-256. |
| `413` | `FILE_TOO_LARGE` | Any file or package exceeds configured limits. |
| `404` | `IMPORT_BATCH_NOT_FOUND` | Requested import batch is unknown. |
| `409` | `IDEMPOTENCY_CONFLICT` | Same `idempotencyKey` is reused with different package bytes or manifest hash. |
| `409` | `IMPORT_BATCH_MODIFIED` | Rollback would delete user-edited data. |
| `429` | `RATE_LIMITED` | Redis rate limit blocked the request. |
| `503` | `DATABASE_REQUIRED` | Postgres is not configured. |
| `503` | `STORAGE_REQUIRED` | Object storage is not configured. |

## 9. Rollback and Idempotency

- Commit is idempotent by `(user_id, idempotencyKey, packageHash)`. Replays of
  the same package return the prior batch summary and mappings with
  `status = "completed"`, not duplicate rows. Reusing the same idempotency key
  for different package bytes or a different manifest hash returns
  `409 IDEMPOTENCY_CONFLICT`.
- Commit is atomic. Any validation, DB, or object-storage failure aborts the
  batch and returns an error; no partial-success status is exposed.
- If object-store upload succeeds but DB insert fails, delete the object before
  returning an error.
- If a later message insert fails, roll back the DB transaction and delete any
  objects written for the batch.
- `DELETE /v1/import/browser/{batchId}` is the user-facing rollback path for an
  already completed batch.
- Rollback must not touch browser-local data. The frontend local-mode data stays
  intact until the user separately clears it.
- `POST /v1/import/browser/preview` is DB-independent and may validate packages
  while the backend is running without `DATABASE_URL`; commit, GET, and rollback
  require Postgres and return `503 DATABASE_REQUIRED` when DB wiring is absent.

## 10. Security Notes

- Treat import packages as untrusted user input.
- Never log manifest bodies, attachment contents, file bytes, or suspected
  secret values.
- Sanitize filenames for display; never use client filenames as object keys.
- ZIP parsing must defend against Zip Slip, decompression bombs, duplicate
  entries, symlinks, and path confusion.
- HTML, tool-call results, plugin outputs, and code blocks are stored as inert
  message content only.
- Object storage remains private; imported file download goes through
  `/v1/files/{fileId}/content`.

# Storage Inventory

This inventory describes current local-first persistence and the target server-backed replacement.

## Current Browser Storage

| Layer | Current Use | Evidence |
|---|---|---|
| `localStorage` | Core settings, provider records, selected models, provider API key envelopes | `src/store/core/coreSettingsStore.ts`, `src/store/storage/storageConfig.ts`, `docs/privacy-and-local-data.md` |
| IndexedDB via `localforage` | Chat metadata, messages, app settings, plugins, skills, assistants, knowledge metadata, memories | `src/store/storage/storageConfig.ts`, `src/store/README.md` |
| OPFS | Uploaded chat files, workspace files, knowledge-base source files | `src/utils/opfs.ts`, `src/store/core/knowledgeStore.ts`, `src/store/core/chatStore.ts` |
| In-memory UI state | transient panels, loading flags, modal state | `src/store/core/uiStore.ts` |

## Current Storage Keys

From `src/store/storage/storageConfig.ts`:

```text
neo-chat-core-settings    localStorage core settings
neo-chat-settings         IndexedDB app settings
neo-chat-storage          IndexedDB chat/session state
knowledge-storage         IndexedDB knowledge metadata
neo-chat-memory           IndexedDB memory state
```

## Current OPFS Areas

From clear/export and store code:

```text
chat
images
knowledge-base
workspaces
```

Current OPFS URLs use the `opfs://` protocol and are resolved by `src/utils/opfs.ts`.

## Target Server Storage

| Data | Target | Reason |
|---|---|---|
| Users, sessions | Postgres | canonical auth/session records |
| Conversations, messages | Postgres | durable structured data and queryability |
| Provider configs | Postgres + encryption | server-side secret boundary |
| File metadata | Postgres | ownership, size, MIME, SHA, storage key |
| File bytes | MinIO/S3-compatible object storage | large binary storage, streamable, backup-friendly |
| Rate limits, cancellation, temp jobs | Redis | fast short-lived state |
| RAG chunks/index metadata | Postgres + vector/index service later | keep optional sidecar isolated |

## File Migration Rule

Do not silently copy browser OPFS files to the server. Migration must be explicit:

```text
User exports local data
  ↓
User previews import contents
  ↓
User confirms upload
  ↓
Go backend validates import
  ↓
Postgres stores metadata; MinIO stores file bodies
```

## Initial Server Tables

```text
users
sessions
conversations
messages
message_attachments
files
provider_configs
audit_logs
```

## File Metadata Contract

```text
id                server-generated UUID/ULID
user_id           owner
filename          original display name
mime_type         validated MIME
size              byte count
sha256            integrity check
storage_backend   local|minio|s3
storage_key       generated key; never raw filename
created_at
deleted_at
```

## Risks

- Existing chat attachments may contain `opfs://` references, base64 data, or remote URLs; import logic must normalize each type.
- Provider secrets currently begin client-owned; server mode must avoid returning plaintext secrets to the browser.
- IndexedDB migrations exist for legacy data; import tooling must reuse/understand normalized shapes rather than assuming only the newest schema.

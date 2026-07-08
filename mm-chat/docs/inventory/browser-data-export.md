# Browser Data Export Inventory

This inventory captures the current local-first export surface that Phase 8 must
consume before server import code is written.

## Source of Truth

Current browser data is split across `localStorage`, IndexedDB via
`localforage`, and OPFS file bytes.

| Area | Current source | Evidence |
| --- | --- | --- |
| Core settings | `localStorage` key `neo-chat-core-settings` | `src/store/storage/storageConfig.ts` |
| App settings | IndexedDB/localforage key `neo-chat-settings` | `src/store/storage/storageConfig.ts` |
| Chat metadata | IndexedDB/localforage key `neo-chat-storage` | `src/store/core/chatStore.ts` |
| Per-session messages | IndexedDB/localforage key `session_messages_{sessionId}` | `src/components/layout/Sidebar.tsx` |
| Knowledge metadata | IndexedDB/localforage key `knowledge-storage` | `src/store/core/knowledgeStore.ts` |
| Memory metadata | IndexedDB/localforage key `neo-chat-memory` | `src/store/core/memoryStore.ts` |
| File bytes | OPFS under `chat/`, `images/`, `knowledge-base/`, `workspaces/` | `src/utils/opfs.ts`, `src/lib/data/appExport.ts` |

## Existing Full-App JSON Export

`src/lib/data/appExport.ts` already creates an app-level JSON envelope:

```ts
interface AppExportPayload {
  exportVersion: 1;
  storageVersion: 4;
  exportedAt: string;
  data: {
    coreSettings?: unknown;
    settings?: unknown;
    chat?: unknown;
    knowledge?: unknown;
    memory?: unknown;
  };
}
```

This export preserves the five main persisted JSON keys but does not enumerate
`session_messages_*` and does not embed OPFS file bytes. It may contain
references such as `opfs://chat/...`, `opfs://images/...`,
`opfs://knowledge-base/...`, and `opfs://workspaces/...`; by itself it is not a
valid server import package.

## Existing Single-Session Export

`src/lib/chat/sessionExport.ts` builds a single session export shaped like the
local `Session` plus `messages` and optional `messageTree`:

```ts
interface SessionExportPayload extends Session {
  messages: Message[];
  messageTree?: SessionMessageTree;
}
```

The sidebar export path loads inactive session messages from
`session_messages_{sessionId}` and active session messages from in-memory chat
state before writing a JSON download.

## Local Chat Shapes Relevant to Import

| Local field | Import meaning |
| --- | --- |
| `Session.id` | Client-only stable ID used for mapping; server generates UUIDs. |
| `Session.title` | Conversation title. |
| `Session.updatedAt` | Milliseconds timestamp; convert to UTC RFC3339. |
| `Session.model` | Parse into provider/model when possible; otherwise keep as metadata. |
| `Session.systemInstruction` | Maps to conversation `system_prompt`. |
| `Session.workspaceId` | Optional grouping metadata; workspaces are not canonical server objects yet. |
| `Message.role = "model"` | Maps to server role `assistant`. |
| `Message.timestamp` | Milliseconds timestamp; convert to `created_at`/`completed_at`. |
| `Message.outputBlocks`, `reasoning`, `usage`, `toolCalls` | Preserve in `output_blocks` or scrubbed metadata; do not execute tools during import. |
| `Attachment.url = opfs://...` | Needs a matching manifest `blobPath` entry and ZIP blob, or a preview warning. |
| `Attachment.data` | Small inline legacy data; convert to file when size is acceptable. |

## OPFS Import Risks

- The current full-app JSON export references OPFS files but does not contain the
  bytes. A server import package must include file blobs inside
  `neo-chat-browser-import-v2.zip`.
- OPFS URLs are browser-local capabilities. They must not be stored in server
  message responses except as scrubbed import metadata for operator debugging.
- Missing OPFS files should block commit for referenced chat attachments unless
  the user explicitly chooses metadata-only import.
- Remote URLs in attachments must not be fetched blindly by the backend; preview
  should report them as remote references and require a later policy decision.
- Provider/API credentials can exist in exported settings. Phase 8 must not
  import plaintext client-owned secrets into server provider configs.

## Phase 8 Export Package Boundary

Phase 8 should create `neo-chat-browser-import-v2.zip` instead of reusing the
current v1 JSON download. Minimum package layout:

```text
neo-chat-browser-import-v2.zip
├── manifest.json
└── files/sha256/{sha256}
```

Exporter requirements:

- Enumerate `appDb.keys()` and include every `session_messages_{sessionId}`
  record; `neo-chat-storage` contains only session/workspace metadata.
- Flush or snapshot the active session before export so in-memory messages are
  not lost.
- Read OPFS bytes for referenced `opfs://` attachments and place them under
  `files/sha256/{sha256}`.
- Decode inline base64/data attachments into file blobs when they should become
  server files.
- Preserve remote URL attachments as metadata only; do not ask the backend to
  fetch them during import.
- Mark knowledge vectors, memories, provider secrets, RAG tokens, plugin auth,
  and voice/search secrets as deferred or rejected; they are not part of the
  core chat import path.

The browser-side exporter should produce a normalized import manifest from the
local JSON/OPFS state before upload. The Go backend should validate only that
normalized manifest plus ZIP file blobs; it should not need to understand every
historic Zustand persistence detail.

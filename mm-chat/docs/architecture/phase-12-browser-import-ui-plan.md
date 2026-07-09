# Phase 12 Browser Import UI Local Plan

## Objective

Build the local browser-to-server migration path before VPS deployment. The
unchanged Next.js/React frontend will generate an explicit
`neo-chat-browser-import-v2.zip`, preview it through the Go backend, commit only
after user confirmation, and expose safe rollback for the imported batch.

## Scope

- Export browser-local chat data from IndexedDB/localforage plus referenced OPFS
  bytes into the Phase 8 ZIP contract.
- Enumerate all `appDb.keys()` records and include every
  `session_messages_{sessionId}` message tree, not only the five-key JSON backup.
- Include OPFS blobs referenced by chat attachments under `files/sha256/{sha256}`.
- Keep provider secrets, plugin auth, RAG tokens, cookies, and local encrypted
  secret envelopes out of the server import package.
- Add a settings UI flow: generate package → preview → confirm commit → refresh
  server sessions → rollback imported batch if still unmodified.
- Use local browser dev through `/mm-api` same-origin proxy; do not require CORS.

## Non-Goals

- No automatic background upload of local browser data.
- No migration of provider configuration or API keys into server provider tables.
- No knowledge vector/RAG import beyond deferred counts/metadata.
- No VPS, auth, multi-user, Kubernetes, or production backup changes in this
  phase.

## Implementation Slices

### 12.1 Exporter Manifest Builder

- Add browser import DTOs and a pure manifest builder under `src/lib/data/`.
- Normalize local `Session` and `Message` data into Phase 8 DTOs.
- Convert local `role = "model"` to import `role = "assistant"`.
- Convert timestamps to UTC RFC3339 and preserve only safe metadata.

Verification: unit tests cover session/message normalization and forbidden-secret
scrubbing.

### 12.2 ZIP Builder and Import API Client

- Use existing `fflate` dependency to generate the ZIP.
- Add server import API shell using multipart `package` field.
- Enable `imports` capability only in configured server mode.

Verification: tests assert ZIP layout and multipart endpoint paths.

### 12.3 Preview UI

- Add a System Settings data-management card for server import preview.
- Before preview, flush the active local session with `syncActiveSession()`.
- Display counts, warnings, errors, and whether commit is allowed.

Verification: local browser can preview a package against `/mm-api`.

### 12.4 Confirmed Commit UI

- Re-submit the exact previewed package only after user confirmation.
- Show returned `batchId`, created counts, and refresh server sessions.

Verification: refresh shows imported conversations/messages from Go state.

### 12.5 Rollback UI

- Add rollback for the last committed batch using
  `DELETE /v1/import/browser/{batchId}`.
- Refresh server sessions after rollback and surface `409 IMPORT_BATCH_MODIFIED`.

Verification: rollback removes the imported batch when unmodified.

### 12.6 Local Smoke

- Run the browser in server mode via `/mm-api`.
- Create one local conversation with an attachment, export/import it, refresh,
  and verify server rendering.
- Record exact commands, URLs, batch IDs, and cleanup notes in `process.md`.

## Rollback

- All browser-local source data remains untouched by import and rollback.
- If frontend changes fail, switch `NEXT_PUBLIC_API_MODE=local` and continue
  using the existing local-first app.
- If backend import data is unwanted, use the batch rollback UI or
  `DELETE /v1/import/browser/{batchId}`; do not wipe Docker volumes unless data
  loss is intended.

## Progress Tracking

`mm-chat/docs/tracking/progress.md` is the source checklist. Mark a slice only
after implementation, targeted verification, and process-log evidence are done.

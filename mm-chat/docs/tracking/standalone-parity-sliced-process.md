# Standalone Parity Sliced Process Log

This is the dedicated process log for the active standalone parity cutover plan:
[`../architecture/standalone-parity-sliced-cutover-plan.md`](../architecture/standalone-parity-sliced-cutover-plan.md).

Use this log for new cutover work instead of burying remaining migration notes
inside the large legacy `process.md`. Keep entries concise, evidence-backed, and
secret-free.

## Process Rule

Every migration group records:

```text
Date / Group / Objective
Changed surfaces
Verification commands and decisive results
Runtime or browser smoke evidence
Residual risks
Next group or rollback note
```

A group is not complete until its targeted tests and smoke are recorded here.
Full-suite gates are reserved for domain cutover, release candidates, and final
clean-copy closure.

Owner commit discipline recorded on 2026-07-15: after each migration group is
implemented, tested, and recorded, create a focused Git commit for that
completed group before starting the next group. Do not batch unrelated future
groups into the same commit.

## 2026-07-15 — G0 Plan Freeze and Guardrails Completed

Objective: collect all remaining unfinished migration work into a new active
plan and establish the rule that each migrated group is tested immediately.

Owner directive:

- organize all remaining unfinished work into a new plan document;
- migrate one group at a time;
- test each group after migration;
- stop treating every small slice as one giant full-suite migration;
- create a new process document and keep the new plan/process as the active
  memory anchors.

Evidence inspected:

```text
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/app/api/**/route.ts
.trellis/tasks/07-07-mm-chat-server-refactor-design/prd.md
```

Remaining blockers captured:

- server-mode UI blockers in `ChatApp.tsx`: regeneration, message version
  switching, assistant presets, message editing, edit branches, message
  deletion, chat deletion, chat duplication, message retraction, smart rename,
  chat renaming, pinning, system instruction editing, and search toggle;
- 25 transitional frontend `/api/*` handlers still registered;
- unfinished domains: Auth UI lifecycle, Teams UI, Knowledge/RAG UI, Plugin
  final ownership, Provider Settings/BYOK, Agent catalogs, Voice, Image, Code
  Execution, Search, parser/RAG/citations, production local-mode removal,
  visual regression, backup/restore, clean-copy, and delete-plan gates.

Documents created or updated:

```text
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
mm-chat/docs/README.md
mm-chat/docs/architecture/README.md
mm-chat/docs/tracking/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Verification completed for this documentation slice:

```text
git diff --check -- mm-chat/docs/...                         # passed
python3 link/path existence checks for the new docs           # passed
rg headings in new plan/process docs                          # passed
```

No runtime code was changed in G0, so frontend/backend tests were intentionally
not run for this documentation-only slice.

Next group: G1 Conversation and Message Operations.

## 2026-07-15 — G1.1 Conversation Metadata Operations Completed

Objective: remove the first server-mode blockers in G1 without changing the
frontend visual baseline.

Completed scope:

- server-backed chat deletion;
- server-backed chat renaming;
- server-backed pin/unpin;
- server-backed system instruction edit/delete.

Changed surfaces:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
PATCH  /v1/chat/conversations/{id}
DELETE /v1/chat/conversations/{id}
```

`PATCH` accepts title, modelRef, systemInstruction, config/metadata merge, and
pinned. `DELETE` soft-deletes the conversation. Frontend server mode now calls
these contracts through typed client/service/store actions and no longer shows
unsupported toasts for chat deletion, chat renaming, pinning, or system
instruction editing.

Verification:

```text
cd mm-chat/backend && go test ./internal/chat                         # passed
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts                  # 4 files / 61 tests passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create -> PATCH title/systemInstruction/pinned/config ->
  list -> DELETE -> list                                               # passed; delete_status=204, listed_after=0
```

Operational note: Compose commands for this stack must include
`--env-file .env.single-server`; running without it falls back to empty Team
cursor keyring defaults and the backend correctly refuses `AUTH_MODE=required`.
No secret values were recorded.

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
message editing
message edit branches
message deletion
chat duplication
message retraction
smart rename
```

Next slice: G1.2 Message deletion and retraction.

## 2026-07-15 — G1.2 Message Deletion and Retraction Completed

Objective: remove the server-mode blockers for deleting a single message and
retracting from a selected message onward, without changing the frontend visual
baseline.

Completed scope:

- server-backed single message deletion;
- server-backed message retraction using `scope=subsequent`;
- frontend server-mode actions now call Go through the typed client/service/
  store path instead of showing unsupported toasts for these two controls.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/errors.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
DELETE /v1/chat/conversations/{conversationId}/messages/{messageId}
DELETE /v1/chat/conversations/{conversationId}/messages/{messageId}?scope=subsequent
```

`scope` is optional. Empty or `message` soft-deletes only the selected message.
`subsequent` soft-deletes the selected message and all later messages by
conversation `sequence_no`. Unknown scopes fail closed with
`INVALID_DELETE_SCOPE`; missing or already-deleted messages map to
`MESSAGE_NOT_FOUND`.

Verification:

```text
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts                  # 4 files / 63 tests passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create conversation -> append 3 messages ->
  DELETE second message -> list first+third ->
  DELETE first message with scope=subsequent -> list empty             # passed; 204/204
```

Smoke identifiers:

```text
conversation=8d517d55-7a7e-42dd-bd0e-612c77643b9f
single delete target=6b57d286-f579-4480-bec1-e51d31c80828
after single delete=7192a3be-760d-4433-a669-1f3c7a43980e,c5cdeabf-8115-4669-92ba-9bdeed9fc47a
after subsequent delete=<empty>
```

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
message editing
message edit branches
chat duplication
smart rename
```

Next slice: G1.3 Message editing and edit branches.

## 2026-07-15 — G1.3 Message Editing and Edit Branches Completed

Objective: remove the server-mode blockers for editing rendered model-message
content and creating edited user-message branches, while keeping the existing
chat UI controls and visual baseline unchanged.

Completed scope:

- server-backed `PATCH` for message content edits;
- frontend server-mode model-message edit now calls Go through the typed
  client/service/store path;
- frontend server-mode user-message edit creates a new persisted branch user
  message instead of mutating the original message;
- server message listing now carries enough parent metadata for the frontend
  server snapshot to reconstruct active branch state.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/lib/chat/types.ts
mm-chat/frontend/src/lib/chat/messageTree.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
PATCH /v1/chat/conversations/{conversationId}/messages/{messageId}
```

The request accepts `{"content":"..."}` only. Empty content fails closed with
`EMPTY_CONTENT`; absent editable fields fail with `NO_MESSAGE_UPDATES`; missing
or deleted messages map to `MESSAGE_NOT_FOUND`. The update clears stale
`output_blocks` so edited model text is rendered from the new canonical
content. Edited user-message branches continue to use
`POST /v1/chat/conversations/{id}/messages` with:

```json
{
  "parentMessageId": "previous-active-parent-when-present",
  "metadata": {
    "branchSourceMessageId": "original-user-message-id",
    "treeParentMessageId": null
  }
}
```

For root-level user branches, `treeParentMessageId:null` is intentional and is
used by the frontend server snapshot to keep sibling branches reconstructable.

Verification:

```text
cd mm-chat/backend && go test ./internal/chat                         # passed
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                   # 6 files / 75 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create conversation -> append root user message ->
  PATCH message content -> append edited root branch with
  branchSourceMessageId/treeParentMessageId metadata -> list messages  # passed
```

Smoke identifiers:

```text
conversation=0a272093-8e18-499f-a489-9245b0220c0b
patched message=2953f2de-7bdf-4945-8a32-948ea3384de0
branch message=bf8e73be-1923-4fd5-950f-cc23d22406bd
patched_content=patched root
patched_outputBlocks=0
branch_parent=<empty>
branch_treeParent=null
listed_ids=2953f2de-7bdf-4945-8a32-948ea3384de0,bf8e73be-1923-4fd5-950f-cc23d22406bd
listed_contents=patched root|edited branch root
```

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
chat duplication
smart rename
```

Next slice: G1.4 Regeneration and message version switching.

## 2026-07-15 — G1.4 Regeneration and Message Version Switching Completed

Objective: remove the server-mode blockers for regenerating an assistant answer
and switching between assistant-message versions, without changing the existing
chat UI layout or interaction language.

Completed scope:

- server-mode regeneration now reuses the selected assistant message's parent
  user message and streams through the existing Go SSE contract instead of
  appending a duplicate user prompt;
- repeated regeneration creates sibling assistant branches under the same user
  message;
- server-mode message version switching updates only the active server read tree
  in memory and does not write browser-local IndexedDB/OPFS state;
- `ChatApp.tsx` no longer shows unsupported toasts for regeneration or message
  version switching.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/handler_test.go

mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/stream
```

No new public backend route was added for this slice. The frontend calls the
existing stream contract with the original parent user message id; the Go chat
runtime already persists the newly streamed assistant message as a later sibling
branch. This keeps regeneration semantics aligned with normal server streaming
and avoids a second mutation path.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts                                      # passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

Provider-cost note: this slice did not run a live external-provider HTTP stream
smoke, because the current Compose stack is configured with real provider
settings and a full regeneration smoke may spend external model quota. The
branching contract is covered by Go mock stream tests, frontend store/component
tests, and Compose readiness/frontend HTTP smoke.

Residual G1 blockers:

```text
assistant presets
chat duplication
smart rename
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G1.5 Chat duplication and assistant presets.

## 2026-07-15 — G1.5 Chat Duplication and Assistant Presets Completed

Objective: remove the server-mode blockers for duplicating chats and applying
assistant presets, while keeping the existing frontend visual baseline and
leaving Agent Catalog server ownership to G2.

Completed scope:

- added server-backed conversation duplication through Go;
- duplicated conversations copy metadata, system instruction, model ref,
  visible messages, message parent links, output blocks, and server attachment
  references;
- duplicated conversations are unpinned by default, and copied assistant
  messages strip operational `runId` metadata so old runs cannot be cancelled
  through the copy;
- server-mode chat duplication now calls the typed API/service/store path and
  loads the copied server snapshot instead of mutating IndexedDB;
- server-mode assistant preset selection now applies the resolved instruction
  to the current empty server conversation or creates a new server conversation;
- `ChatApp.tsx` no longer shows unsupported toasts for assistant presets or
  chat duplication.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/duplicate
```

Request body accepts optional `title` and `idempotencyKey`. If `title` is
omitted, Go uses `<source title> (Copy)`. Message IDs are regenerated and
parent-message links are remapped inside the copied conversation. Server file
attachment records are linked to the copied messages; file bytes are not
re-uploaded.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                          # 6 files / 78 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

HTTP smoke identifiers:

```text
source=d27fd74d-198c-4c59-9df8-47a9bd3e068d
duplicate=0489beee-1ed7-4677-96f2-a7a07693a2c6
sourceMessage=f28e0ff0-9fd6-46ef-8e8f-ce404b36cab3
duplicateTitle=G1.5 duplicate smoke (Copy)
duplicatePinned=false
duplicateMessageCount=1
listedMessageCount=1
listedContent=copy me
frontendBytes=96436
```

Residual G1 blockers:

```text
smart rename
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G1.6 Smart rename / title generation through server-owned route.

## 2026-07-15 — G1.6 Smart Rename and Server-Owned Title Generation Completed

Objective: remove the final G1 server-mode blocker by moving smart rename/title
generation behind a Go-owned route that reads server conversation history.

Completed scope:

- added `POST /v1/chat/conversations/{conversationId}/title` in Go;
- the title route reads messages from Postgres and builds the title prompt on
  the server side;
- if no `modelRef` or provider is available, the route returns a normalized
  first-user-message fallback without spending external model quota;
- frontend server API/client/service/store now exposes server title generation;
- server-mode smart rename calls the Go route and updates the conversation title
  through the existing `PATCH /v1/chat/conversations/{id}` path;
- server-mode auto-title after the first streamed message uses the same Go route
  and only applies the title while the conversation title is still `New Chat`;
- `ChatApp.tsx` no longer shows an unsupported toast for smart rename.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/title
```

Request body accepts optional `modelRef`. With `modelRef`, Go may call the
configured provider through the existing provider abstraction. Without
`modelRef`, Go returns the deterministic first-user-message fallback; the smoke
used this path to avoid external model cost.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                          # 6 files / 79 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

HTTP smoke identifiers:

```text
conversation=6d2b8d8d-3954-4092-a77a-025140e832bb
message=1fbb866d-8b64-4802-9d8b-de8e3e0e32c8
title=Server title fallback smoke
updatedTitle=Server title fallback smoke
frontendBytes=96436
```

Residual G1 blockers:

```text
<none>
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G2 Title, Related Questions, and Agent/Assistant Catalogs.

## 2026-07-15 — G2 Related Questions and Agent/Assistant Catalogs Completed

Objective: replace the remaining G2 helper-generation and catalog dependencies
with server-owned contracts while keeping the existing UI surface unchanged.

Completed scope:

- added `POST /v1/chat/conversations/{conversationId}/related-questions` in
  Go;
- related-question generation now reads the conversation messages from Postgres
  and uses the latest user/assistant pair, instead of accepting browser-owned
  `history` or provider config;
- the related-question route returns `{ "questions": [] }` without external
  model cost when no `modelRef`, provider, or usable message pair is present;
- added a Go-owned Agent catalog service and routes:
  - `GET /v1/agents?locale=en|zh|ja`;
  - `GET /v1/agents/{identifier}?locale=en|zh|ja`;
- catalog list/detail responses are normalized server-side and invalid agent
  identifiers fail before any upstream registry request;
- frontend API client now exposes `agents` and server `relatedQuestions`
  methods;
- frontend `agentService` routes server-mode catalog list/detail through Go
  `/v1/*` routes and keeps the legacy Next `/api/agents*` path only for local
  mode rollback;
- server-mode post-generation related prompts now call the Go conversation
  route with the server conversation ID and no browser history payload;
- server mode fails closed with an empty related-question list when no
  conversation ID is available, instead of falling back to the transitional
  Next route.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/agents/types.go
mm-chat/backend/internal/agents/service.go
mm-chat/backend/internal/agents/handler.go
mm-chat/backend/internal/agents/handler_test.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/metrics.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/mode.ts
mm-chat/frontend/src/services/api/client/local/agentApi.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/agentApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/agentService.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/agentService.test.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime contracts:

```text
POST /v1/chat/conversations/{conversationId}/related-questions
GET  /v1/agents?locale=en|zh|ja
GET  /v1/agents/{identifier}?locale=en|zh|ja
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/agents ./internal/chat ./internal/httpserver            # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/agentService.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                         # 7 files / 69 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                              # passed
cd mm-chat/frontend && corepack pnpm format:check                           # passed
cd mm-chat/frontend && corepack pnpm lint                                   # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                # passed
curl -fsS http://127.0.0.1:8080/ready                                       # ready
curl -fsS http://127.0.0.1:18080 | wc -c                                    # 96436
```

HTTP smoke identifiers:

```text
conversation=d5b90cc2-f74e-46e3-8698-6ad37d10532d
message=39dee7c2-2f54-4396-a161-3735b86766df
relatedQuestions=[]
agentsStatus=200
agentsUnavailable=false
agentsCount=500
frontendBytes=96436
```

Residual G2 blockers:

```text
<none>
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G3 Auth, Runtime Config, Provider Settings, and BYOK.

## 2026-07-15 — G3.1 Runtime Config, Provider Model, BYOK Route Boundary Completed

Objective: open the first G3 server-owned runtime boundary before wiring UI:
Go owns public runtime config, server-default provider model listing, BYOK
public-key publication, and the frontend API client exposes the corresponding
local/server adapters.

Completed scope:

- added Go `runtimeconfig` service and handler;
- registered public `GET /v1/config` and `GET /v1/byok/public-key` routes;
- registered protected `POST /v1/providers/models` route;
- `GET /v1/config` publishes browser-safe provider/default deployment facts
  without serializing provider API keys;
- `POST /v1/providers/models` supports only `source:"server-default"` model
  lists from server config in this slice;
- plaintext provider secrets are rejected and custom BYOK provider model refresh
  remains fail-closed for a later G3 slice;
- frontend API client now has `auth`, `settings`, `providers`, and `byok`
  subclients with local rollback and server `/v1/*` shells;
- server Auth shell routes login/logout/me/invite/recovery/session-revoke to Go
  auth endpoints and sends Bearer only through explicit token input.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/runtimeconfig/types.go
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/runtimeconfig/handler.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/backend/internal/runtimeconfig/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/http.ts
mm-chat/frontend/src/services/api/client/local/authApi.ts
mm-chat/frontend/src/services/api/client/local/settingsApi.ts
mm-chat/frontend/src/services/api/client/local/providerApi.ts
mm-chat/frontend/src/services/api/client/local/byokApi.ts
mm-chat/frontend/src/services/api/client/server/authApi.ts
mm-chat/frontend/src/services/api/client/server/settingsApi.ts
mm-chat/frontend/src/services/api/client/server/providerApi.ts
mm-chat/frontend/src/services/api/client/server/byokApi.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime contracts opened in this slice:

```text
GET  /v1/config
POST /v1/providers/models          # server-default source only
GET  /v1/byok/public-key
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/runtimeconfig ./internal/config ./internal/httpserver  # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...           # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts \
  src/__tests__/envExample.test.ts \
  src/__tests__/serverDefaults.test.ts                                     # 6 files / 80 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                             # passed
cd mm-chat/frontend && corepack pnpm format:check                          # passed
cd mm-chat/frontend && corepack pnpm lint                                  # passed
```

Residual G3 blockers:

```text
G3.2 frontend Auth lifecycle UI still calls the legacy local access-password route.
G3.3 ChatApp and ProviderSettings still call transitional /api/config,
     /api/providers/models, and /api/byok/public-key directly outside the new
     API client boundary.
G3 custom provider BYOK decrypt/model refresh is fail-closed in Go until the UI
   adapter and secret-envelope handling slice.
```

Next slice: G3.2 Frontend Auth lifecycle wired to Go login/logout/me.

## 2026-07-15 — G3.2 Frontend Auth Lifecycle Gate Completed

Objective: wire frontend server-mode Auth lifecycle to Go login/logout/me while
preserving the local access-password rollback path.

Completed scope:

- added a browser `sessionStorage`-backed server auth session helper for the Go
  Bearer token returned by `POST /v1/auth/login`;
- server-mode HTTP client now injects `Authorization: Bearer <token>` from that
  runtime session into `/v1/*` requests;
- added `ServerAuthGate`, which verifies an existing token with `GET /v1/me`
  before mounting `ChatApp` and clears stale sessions on auth failure;
- `app/page.tsx` routes to `ServerAuthGate` only when
  `NEXT_PUBLIC_API_MODE=server` and `AUTH_MODE=required`;
- `AccessPasswordPage` retains the existing local `/api/access/verify` flow and
  adds a server-auth mode that sends `{ email, password }` to Go login;
- frontend Compose/env examples now expose `AUTH_MODE` to the frontend runtime
  so the SSR page can select the correct gate.

Changed surfaces for this slice:

```text
mm-chat/compose.single-server.yml
mm-chat/frontend/.env.example
mm-chat/frontend/src/app/page.tsx
mm-chat/frontend/src/components/app/AccessPasswordPage.tsx
mm-chat/frontend/src/components/app/ServerAuthGate.tsx
mm-chat/frontend/src/i18n/locales/en/AccessPassword.json
mm-chat/frontend/src/i18n/locales/ja/AccessPassword.json
mm-chat/frontend/src/i18n/locales/zh/AccessPassword.json
mm-chat/frontend/src/lib/security/serverAuthMode.ts
mm-chat/frontend/src/services/api/client/authSession.ts
mm-chat/frontend/src/services/api/client/server/httpClient.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/envExample.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime flow:

```text
server mode + AUTH_MODE=required -> ServerAuthGate
ServerAuthGate existing token -> GET /v1/me -> ChatApp or clear session
ServerAuthGate login form -> POST /v1/auth/login -> sessionStorage token -> ChatApp
server API calls -> Authorization: Bearer <token>
local mode/access-password -> unchanged /api/access/verify + httpOnly cookie
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/envExample.test.ts \
  src/__tests__/accessControl.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                    # passed, 6 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                         # passed
cd mm-chat/frontend && corepack pnpm format:check                      # passed
cd mm-chat/frontend && corepack pnpm lint                              # passed
```

Residual G3 blockers:

```text
G3.3 ChatApp and ProviderSettings still call transitional /api/config,
     /api/providers/models, and /api/byok/public-key directly outside the new
     API client boundary.
G3.4 hosted/dev auth behavior and same-origin Compose smoke still pending.
```

Next slice: G3.3 Provider Settings/BYOK UI adapters through the API client.

## 2026-07-15 — G3.3 Provider Settings and BYOK API-Client Wiring Completed

Objective: remove direct server-mode UI calls to transitional Next runtime
config/provider/BYOK routes and route those flows through the API client
boundary.

Completed scope:

- `ChatApp` runtime config bootstrap now calls
  `createNeoChatApiClient().settings.getRuntimeConfig()`;
- `ChatApp` default server-provider model bootstrap now calls
  `createNeoChatApiClient().providers.listModels()`;
- `ProviderSettings` model refresh now calls the provider API client instead of
  posting directly to `/api/providers/models`;
- BYOK public-key loading now calls `createNeoChatApiClient().byok.getPublicKey()`;
- local adapter implementations remain the only code paths that call
  `/api/config`, `/api/providers/models`, or `/api/byok/public-key` for local
  rollback.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/components/settings/ProviderSettings.tsx
mm-chat/frontend/src/lib/byok/client.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/byok.test.ts \
  src/__tests__/serverDefaults.test.ts \
  src/__tests__/envExample.test.ts                                  # passed, 5 files / 84 tests
cd mm-chat/frontend && corepack pnpm typecheck                      # passed
cd mm-chat/frontend && corepack pnpm format:check                   # passed
cd mm-chat/frontend && corepack pnpm lint                           # passed
rg direct transitional config/provider/BYOK calls                   # only local adapters remain
```

Residual G3 blockers:

```text
G3.4 hosted/dev auth behavior and same-origin Compose smoke still pending.
```

Next slice: G3.4 Hosted/dev auth behavior and same-origin smoke.

## 2026-07-15 — G3.4 Hosted/Dev Auth Smoke Completed

Objective: verify hosted/dev auth behavior and same-origin `/mm-api` runtime
routing after the G3 auth/config/provider/BYOK wiring.

Completed scope:

- ran Compose build/start for backend and frontend with `.env.single-server`;
- verified current dev auth mode from `.env.single-server` exposes runtime config
  and allows unauthenticated chat reads as expected for development mode;
- attempted `AUTH_MODE=required` smoke and found backend startup failed because
  required auth also requires Team cursor keyring settings;
- added explicit `local-dev` Team cursor keyring defaults to the single-server
  Compose/backend env examples so required-mode local smoke can start;
- reran `AUTH_MODE=required` Compose smoke and verified same-origin `/mm-api`
  config stays public while chat routes return `401 UNAUTHENTICATED` without a
  Bearer token;
- verified the frontend home page renders the client AuthGate shell in required
  mode before `ChatApp` mounts;
- restored the original `.env.single-server` stack after the required-mode smoke.

Changed surfaces for this slice:

```text
mm-chat/compose.single-server.yml
mm-chat/backend/.env.example
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend             # passed
GET http://127.0.0.1:8080/ready                                          # 200
GET http://127.0.0.1:18080/                                              # 200, bytes=96504
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=local
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 200 in dev mode

AUTH_MODE=required docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --force-recreate backend frontend    # passed after local-dev cursor keyring default
GET http://127.0.0.1:18080/                                              # 200, contains "Checking session"
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=hosted
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 401 UNAUTHENTICATED

docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d backend frontend                     # restored original stack
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=local
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 200 after restore

cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/envExample.test.ts \
  src/__tests__/apiClientScaffold.test.ts                                # passed, 2 files / 46 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
```

Residual G3 blockers:

```text
<none>
```

Next slice: G4 Plugin Registry, Install, and Execution Final Ownership.

## 2026-07-15 — G4.1 Server Plugin Tool Planning Boundary Completed

Objective: land the first plugin slice without taking all plugin ownership in
one bite: provider-side tool planning goes through Go, while browser plugin
execution remains bounded and explicitly transitional.

Completed scope:

- added `orchestrateServerPlugins` as the frontend server-mode bridge from
  active plugin selections to Go `/v1/chat/tools/plan`;
- offered only installed, active, enabled plugin functions to Go and failed
  closed on duplicate function names or unoffered planned calls;
- kept plugin auth values out of the Go planning request;
- executed planned calls sequentially through the existing hardened plugin
  execution helper and retained `success|error` status per call;
- appended plugin results as explicitly untrusted context capped at 64 KiB before
  the final Go chat stream;
- covered URL/body mapping and malformed successful response handling for the
  API-client `/v1/chat/tools/plan` adapter;
- split G4 into smaller remaining slices so registry, install, execute final
  ownership, and live smoke can each be migrated and tested separately.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/serverPluginOrchestration.ts
mm-chat/frontend/src/__tests__/serverPluginOrchestration.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/serverPluginOrchestration.test.ts  # passed, 1 file / 9 tests
cd mm-chat/frontend && corepack pnpm typecheck     # passed
cd mm-chat/frontend && corepack pnpm format:check  # passed
cd mm-chat/frontend && corepack pnpm lint          # passed
```

Residual G4 blockers:

```text
G4.2 plugin registry/list adapter
G4.3 plugin install/custom-manifest adapter
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.2 Plugin registry/list adapter.

## 2026-07-15 — G4.2 Plugin Registry/List Adapter Completed

Objective: migrate the plugin marketplace list read as its own bounded slice,
without mixing install or execution final ownership into the same change.

Completed scope:

- added `PluginApi.listAvailable()` to the frontend API-client contract;
- added a local adapter that preserves the existing `/api/plugins/list` rollback
  path only inside `client/local/pluginApi.ts`;
- added a server adapter that targets Go `/v1/plugins` and treats a missing or
  unavailable registry route as explicit `{ plugins: [], unavailable: true }`;
- routed `fetchApiGuruList()` through `createNeoChatApiClient().plugins` instead
  of direct component/service fetches;
- covered default local cache behavior, server-mode URL routing, unavailable
  registry degradation, and malformed successful server responses;
- confirmed direct `/api/plugins/list` usage is now limited to the local adapter,
  route tests, and static route constants.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/pluginService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginService.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginService.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts          # passed, 5 files / 67 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
rg '"/api/plugins/list"|/api/plugins/list' mm-chat/frontend/src -n
# remaining direct route references: local adapter, static route constant, tests
```

Residual G4 blockers:

```text
G4.3 plugin install/custom-manifest adapter
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.3 Plugin install/custom-manifest adapter.

## 2026-07-15 — G4.3 Plugin Install Adapter Completed

Objective: migrate plugin install and custom manifest install calls as a separate
slice, without claiming final plugin execution ownership.

Completed scope:

- extended `PluginApi` with `install({ plugin | customInput })`;
- kept the legacy `/api/plugins/install` route reachable only through the local
  API-client adapter for rollback/local mode;
- added a server adapter that targets Go `/v1/plugins/install` and converts a
  missing/unavailable route into recoverable `PLUGIN_INSTALL_UNAVAILABLE`;
- routed `installPlugin()` and `installCustomPlugin()` through the API client;
- verified server mode does not fall back to the Next install route when Go has
  no plugin install endpoint yet.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/pluginService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginService.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginService.test.ts \
  src/__tests__/apiClientScaffold.test.ts  # passed, 2 files / 56 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
rg '"/api/plugins/install"|/api/plugins/install' mm-chat/frontend/src -n
# remaining direct route references: local adapter, static route constant, tests
```

Residual G4 blockers:

```text
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.4 Plugin execute final ownership.

## 2026-07-15 — G4.4 Plugin Execute API-Client Boundary Completed

Objective: centralize plugin execution behind the API client without breaking the
G4.1 server planning flow that still depends on the hardened transitional Next
execution route.

Completed scope:

- extended `PluginApi` with `execute({ payload })`;
- moved direct `/api/plugins/execute` fetch construction into
  `client/pluginExecutionHttp.ts`;
- routed `executePluginFunction()` through `createNeoChatApiClient().plugins`
  while preserving BYOK retry semantics by rebuilding the encrypted payload for
  each retry;
- kept both local and server adapters on the same bounded transitional execution
  route for this slice, matching the current Server Plugin Orchestration
  contract;
- added API-client coverage proving server-mode plugin execution uses the
  isolated transitional adapter rather than the Go `/mm-api` prefix;
- reclassified the remaining work so final route retirement is a separate G4.5
  slice instead of being hidden inside this adapter-boundary slice.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/client/pluginExecutionHttp.ts
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts # passed, 3 files / 61 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
```

Residual G4 blockers:

```text
G4.5 plugin execute final ownership and transitional route retirement
G4.6 live browser smoke
```

Next slice: G4.5 Plugin execute final ownership and transitional route retirement.

## 2026-07-15 — G4.5a Go Plugin Execution Fail-Closed Admission Completed

Objective: remove production server-mode fallback to the transitional Next plugin
execution route without pretending the full Go plugin sandbox exists yet.

Completed scope:

- added Go `internal/plugins` handler with explicit routes:
  - `GET /v1/plugins` returns an empty unavailable registry response;
  - `POST /v1/plugins/install` fails closed with `PLUGIN_INSTALL_UNAVAILABLE`;
  - `POST /v1/plugins/execute` fails closed with
    `PLUGIN_EXECUTION_UNAVAILABLE`;
- registered `/v1/plugins` and `/v1/plugins/*` in the Go HTTP server and metrics
  path normalizer;
- kept `GET /v1/plugins` public for marketplace visibility, while install and
  execute stay behind normal auth middleware in required mode;
- changed the frontend server plugin adapter so server-mode execution posts to
  Go `/v1/plugins/execute` and no longer falls back to `/api/plugins/execute`;
- kept `/api/plugins/execute` only in the local adapter rollback path;
- updated the API-client contract and inventory to reflect the Go fail-closed
  execution boundary.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                         # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                  # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                     # passed
cd mm-chat/frontend && corepack pnpm format:check                  # passed
cd mm-chat/frontend && corepack pnpm lint                          # passed
```

Residual G4 blockers:

```text
G4.5b plugin execute sandbox implementation
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5b Plugin execute sandbox implementation.

## 2026-07-15 — G4.5b Minimal Go Plugin Execution Sandbox Completed

Objective: turn the Go plugin execution gate from pure fail-closed admission into
a minimal safe executor while preserving one-slice-at-a-time migration. This
slice intentionally does not claim persistent registry ownership yet.

Completed scope:

- changed Go BYOK public-key algorithm metadata to the frontend envelope contract
  `RSA-OAEP-256+A256GCM`;
- added Go BYOK `DecryptOptionalSecret` / `DecryptSecretEnvelope` support and
  wired plugin execution to the same runtime config service instance so
  ephemeral development keys do not drift;
- implemented Go `/v1/plugins/execute` for full manifest payloads:
  - validates the selected function is declared by the supplied plugin;
  - substitutes path parameters and appends GET query args;
  - rejects plaintext plugin auth;
  - decrypts BYOK `valueSecret` using `plugin:{pluginId}:auth` context;
  - applies bearer/oauth2/apiKey auth to header/query/body according to config;
  - blocks localhost/private/link-local outbound URLs and redirects by default;
  - enforces HTTP method allowlist, timeout, 2 MiB response cap, and generic JSON/text result normalization;
- kept id-only plugin execution fail-closed with `PLUGIN_REGISTRY_REQUIRED` until
  the registry-backed finalization slice;
- changed server-mode frontend plugin execution to send the full plugin manifest
  payload to Go; local mode still keeps `/api/plugins/execute` as rollback only.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/runtimeconfig ./internal/plugins ./internal/httpserver       # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                         # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
git diff --check -- mm-chat                                               # passed
```

Residual G4 blockers:

```text
G4.5c registry-backed plugin execute finalization
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c Registry-backed plugin execute finalization, or G4.6 live smoke
if the owner accepts full-manifest execution as the smoke baseline before
registry persistence.

## 2026-07-15 — G4.5c.1 Go Registry Id-only Bridge Completed

Objective: move server-mode plugin execution off full-manifest production
payloads without swallowing the full durable-registry scope in one slice.

Completed scope:

- added a Go plugin registry interface with an in-memory implementation seeded
  by the current built-in plugin definitions;
- changed `POST /v1/plugins/install` from pure fail-closed to register supplied
  plugin payloads in the Go registry and return the installed plugin;
- kept custom OpenAPI manifest install fail-closed with
  `PLUGIN_CUSTOM_INSTALL_UNAVAILABLE` until the durable conversion slice;
- changed id-only `POST /v1/plugins/execute` to resolve
  `pluginId/functionName` from the Go registry, while preserving full-manifest
  execution as compatibility;
- changed server-mode frontend plugin execution to send id-only payloads to Go
  and never fall back to `/api/plugins/execute`;
- preserved G4.5b sandbox controls: BYOK encrypted auth, private URL/redirect
  blocking, timeout, response cap, and generic result normalization.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/builtins.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                              # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                       # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2 Postgres-backed plugin registry persistence
G4.5c.2 Custom OpenAPI manifest conversion in Go
G4.5c.2 Plugin audit metadata and built-in result normalizers
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2 durable registry completion, kept separate so the migration
continues one tested group at a time.

## 2026-07-15 — G4.5c.2a Postgres Plugin Registry Persistence Completed

Objective: make the Go plugin registry durable for installed plugin payloads
without mixing in custom OpenAPI manifest conversion or built-in result
normalizers.

Completed scope:

- added migration `011_plugin_registry` with `plugin_registry` JSONB payload
  storage, installing-user audit reference, built-in flag, timestamps, and
  rollback SQL;
- added a Go `PostgresRegistry` implementing save/get/list over the durable
  table while overlaying built-in plugin definitions as authoritative entries;
- wired `cmd/api` to use the Postgres registry whenever `DATABASE_URL` provides
  a SQL DB, while local/dev without DB keeps the memory registry fallback;
- changed `GET /v1/plugins` to list Go registry plugins instead of returning an
  unavailable empty registry;
- rejected installed plugin attempts that reuse built-in ids with
  `PLUGIN_ID_RESERVED`;
- updated runtime public deployment health so a database-backed plugin registry
  reports as a shared store.

Changed surfaces for this slice:

```text
mm-chat/backend/migrations/011_plugin_registry.up.sql
mm-chat/backend/migrations/011_plugin_registry.down.sql
mm-chat/backend/internal/migration/plugin_registry_schema_test.go
mm-chat/backend/internal/plugins/repository_postgres.go
mm-chat/backend/internal/plugins/repository_postgres_test.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/backend/cmd/api/main.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed with escalated httptest port permission
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts \
  src/__tests__/serverDefaults.test.ts                                  # passed, 4 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2c built-in result normalizers or, if the owner wants runtime
confidence first, G4.6 smoke against a converted plugin.

## 2026-07-15 — G4.5c.2b Go OpenAPI Plugin Install Conversion Completed

Objective: move custom OpenAPI manifest conversion into the Go backend as its
own tested slice, without mixing in audit metadata cleanup or built-in result
normalizers.

Completed scope:

- added a Go OpenAPI/Swagger converter that reads `servers` or
  `host/schemes/basePath`, maps supported HTTP operations into plugin
  functions, carries path/query parameter schemas, and preserves apiKey/bearer
  auth declarations;
- changed `POST /v1/plugins/install` to accept raw custom OpenAPI JSON,
  bounded manifest URL fetches, and marketplace payloads with `manifestUrl` plus
  empty `functions`;
- routed manifest URL fetches through the Go outbound URL policy and redirect
  checks, with a 3 MiB manifest response cap and explicit install error codes;
- kept supplied full plugin payload registration intact, then registered
  converted plugins into the same memory/Postgres registry so server-mode
  execution can continue with `pluginId/functionName` only;
- updated route tests so custom manifest install is no longer fail-closed.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/openapi.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                              # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver ./internal/runtimeconfig \
  ./internal/migration ./cmd/api                                        # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts \
  src/__tests__/serverDefaults.test.ts                                  # passed, 4 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2c built-in result normalizers or G4.6 live smoke against a
converted plugin.

## 2026-07-15 — G4.5c.2c Go Built-in Plugin Result Normalizers Completed

Objective: move built-in plugin result normalization into Go as a narrow slice,
without touching audit metadata or live-smoke wiring.

Completed scope:

- normalized Jina Web Reader `{code:200,data.content}` payloads into readable
  markdown strings inside Go `/v1/plugins/execute`;
- normalized Agnes image responses into `{imageUrl,imageBase64,revisedPrompt,raw}`
  envelopes;
- normalized Agnes video status/result payloads into stable task/video/status,
  generation status, progress, media URL, error, and raw fields;
- normalized Unsplash `results[]` payloads into the compact image result array
  shape already expected by the frontend fallback path;
- kept local Next plugin execution normalizers unchanged for rollback/local mode.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/normalizers.go
mm-chat/backend/internal/plugins/normalizers_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/plugins # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...              # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginResponseNormalizers.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                              # passed, 3 files / 17 tests
cd mm-chat/frontend && corepack pnpm typecheck                                  # passed
cd mm-chat/frontend && corepack pnpm format:check                               # passed
cd mm-chat/frontend && corepack pnpm lint                                       # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: decide whether to add the remaining audit metadata or run G4.6 live
smoke first.

## 2026-07-15 — G4.6a Zero-cost Plugin Orchestration Smoke Harness Completed

Objective: add a reproducible smoke harness for the plugin final-ownership path
without spending external provider quota or relying on public plugin services.
This is intentionally not marked as the final browser/provider live smoke.

Completed scope:

- added an in-process backend smoke test that mounts real Go chat and plugin HTTP
  handlers on one mux;
- installs a custom OpenAPI weather plugin through `POST /v1/plugins/install`;
- plans a plugin call through `POST /v1/chat/tools/plan` using a fake provider;
- executes the installed plugin through id-only `POST /v1/plugins/execute` using
  a fake plugin HTTP transport;
- builds bounded untrusted plugin context and sends it through
  `POST /v1/chat/conversations/{id}/stream`;
- verifies the Go SSE stream completes and the assistant message is persisted
  with the plugin-derived answer.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/plugin_orchestration_smoke_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat
# passed with escalated httptest loopback permission
```

Residual G4 blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6b live browser/provider smoke with a real plugin result
```

Next slice: either define the remaining plugin audit metadata contract, or run
G4.6b once approved credentials/runtime are available.

## 2026-07-15 — G6.1 Server-mode Media Job Fail-closed Gates Completed

Objective: start G6 with a narrow frontend/server boundary slice. Do not enable
real voice, image-generation, or code-execution jobs yet; only prevent
server-mode fallthrough to transitional Next routes.

Completed scope:

- added disabled `voice`, `imageGeneration`, and `codeExecution` capability
  flags to the frontend API-client capability map;
- gated `chatService.executeCode()` in server mode so it returns an explicit
  unsupported error string instead of calling `/api/chat/execute-code`;
- gated `chatService.generateImage()` in server mode so it throws an explicit
  unsupported feature error instead of calling `/api/chat/generate-image`;
- gated `voiceService.transcribeAudio()` and non-browser
  `voiceService.synthesizeSpeech()` in server mode so they throw explicit
  unsupported feature errors instead of calling `/api/voice/*`;
- left browser-native speech recognition/synthesis behavior local-only and
  unchanged.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/mode.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/services/api/voiceService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                       # passed, 5 files / 71 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
git diff --check -- mm-chat                                               # passed
```

Residual G6 blockers:

```text
G6.2 Voice synthesis/transcription Go job admission
G6.3 Image generation Go job admission
G6.4 Code execution Go job admission
G6.5 Job audit/rate-limit/cancel metadata and provider smoke
```

Next slice: G6.2 voice job admission contract, unless the owner chooses image
or code admission first.

## 2026-07-15 — G6.2 Voice Job Admission Routes Completed

Objective: add Go-owned voice job admission endpoints without enabling real
speech-to-text or text-to-speech execution yet. The goal is a typed,
fail-closed server boundary that can later receive executors, storage, audit,
rate-limit, and cancellation logic.

Completed scope:

- added `internal/voicejobs` with request/response DTOs, a fail-closed service,
  and a handler for `POST /v1/voice/transcribe` and
  `POST /v1/voice/synthesize`;
- `transcribe` validates multipart admission shape, required audio part, and
  supported provider identifiers before returning `VOICE_JOBS_UNAVAILABLE`;
- `synthesize` validates strict JSON, required text, and supported provider
  identifiers before returning `VOICE_JOBS_UNAVAILABLE`;
- registered both routes in the Go HTTP server and added metric-path
  normalization so the endpoints do not collapse into `__unknown__`;
- kept frontend `voice` capability disabled, so server-mode UI still fails
  closed from G6.1 until real execution is implemented.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/voicejobs/types.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/voicejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.3 Image generation Go job admission
G6.4 Code execution Go job admission
G6.5 Real voice executors, output storage, audit/rate-limit/cancel metadata,
and provider smoke
```

Next slice: G6.3 image-generation Go job admission, keeping the same
fail-closed pattern.

## 2026-07-15 — G6.3 Image Generation Admission Route Completed

Objective: add a Go-owned image-generation admission endpoint without enabling
real image generation, provider calls, object storage writes, or billing/audit
side effects.

Completed scope:

- added `internal/imagejobs` with server-only `modelRef + prompt` request DTOs,
  response DTOs, a fail-closed service, and a handler for
  `POST /v1/images/generations`;
- rejected legacy-style plaintext provider objects via strict JSON decoding;
- validated required `modelRef.providerId`, `modelRef.modelId`, prompt, prompt
  length, and image count before returning `IMAGE_JOBS_UNAVAILABLE`;
- registered `/v1/images/generations` in the Go HTTP server and metric-path
  normalizer;
- kept frontend `imageGeneration` capability disabled until real execution,
  storage, and audit controls are added.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/imagejobs/types.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/imagejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.4 Code execution Go job admission
G6.5 Real voice/image executors, output storage, audit/rate-limit/cancel
metadata, and provider smoke
```

Next slice: G6.4 code-execution admission, still fail-closed and sandbox-first.

## 2026-07-15 — G6.4 Code Execution Admission Route Completed

Objective: add a Go-owned code-execution admission endpoint without enabling
model-simulated execution, local sandbox execution, filesystem access, network
access, or billing/audit side effects.

Completed scope:

- added `internal/codejobs` with server-only `modelRef + language + code`
  request DTOs, response DTOs, a fail-closed service, and a handler for
  `POST /v1/code/executions`;
- rejected legacy-style plaintext provider objects via strict JSON decoding;
- validated required `modelRef.providerId`, `modelRef.modelId`, non-empty code,
  maximum code length, and supported language before returning
  `CODE_EXECUTION_UNAVAILABLE`;
- preserved original code text after validation so a future sandbox receives the
  exact submitted program rather than a trimmed copy;
- registered `/v1/code/executions` in the Go HTTP server and metric-path
  normalizer;
- kept frontend `codeExecution` capability disabled until a real sandbox,
  storage/audit, rate-limit, cancellation, and smoke path exist.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/codejobs/types.go
mm-chat/backend/internal/codejobs/service.go
mm-chat/backend/internal/codejobs/handler.go
mm-chat/backend/internal/codejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/codejobs ./internal/httpserver                          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5 Real voice/image/code executors, output storage, audit/rate-limit/cancel
metadata, sandbox policy, and provider smoke
```

Next slice: G6.5 executor/storage/audit design split; do not enable real code
execution without an explicit sandbox contract.

## 2026-07-15 — G6.5a Sanitized Job Admission Audit Completed

Objective: add a shared audit metadata seam for voice, image-generation, and
code-execution job admission without enabling real execution, storage writes,
rate limits, cancellation, or provider smoke.

Completed scope:

- added `internal/jobaudit` with job kind/status constants, sanitized event DTOs,
  recorder interface, recorder function adapter, user-id attachment from auth
  context, and recorder-failure wrapping;
- wired voice, image, and code fail-closed services to emit unavailable audit
  events before returning their existing unavailable errors;
- audit events include only `kind`, `status`, `userId`, `providerId`, `modelId`,
  `language`, and `reason`;
- audit events intentionally do not contain prompt text, submitted source code,
  synthesis text, or audio bytes;
- audit sink failure maps to `503 JOB_AUDIT_UNAVAILABLE`, preserving fail-closed
  behavior for future enabled executors.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobaudit/jobaudit.go
mm-chat/backend/internal/jobaudit/jobaudit_test.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/service_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/service_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/codejobs/service.go
mm-chat/backend/internal/codejobs/service_test.go
mm-chat/backend/internal/codejobs/handler.go
mm-chat/backend/internal/codejobs/handler_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/jobaudit ./internal/codejobs ./internal/imagejobs \
  ./internal/voicejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5b Shared job rate-limit and cancellation gates
G6.5c Real voice/image executors with output storage and provider smoke
G6.5d Code execution sandbox contract before any real executor is enabled
```

Next slice: G6.5b shared job rate-limit/cancel gate, still without enabling
real executors.

## 2026-07-15 — G6.5b Job Cancellation and Rate-limit Gate Completed

Objective: add the shared job-control boundary needed by future async
voice/image/code executors without enabling any real executor or cancellation
state mutation yet.

Completed scope:

- added `internal/jobcontrol` with `POST /v1/jobs/{jobId}/cancel` route parsing,
  job-id validation, and fail-closed service behavior;
- invalid job ids and unknown job-control subroutes return `404 NOT_FOUND`
  without echoing the raw identifier;
- valid cancellation requests return `501 JOB_CANCELLATION_UNAVAILABLE` until
  a durable job registry/cancellation store exists;
- registered `/v1/jobs/{jobId}/cancel` in the Go HTTP server and metric-path
  normalizer;
- added an HTTP-server regression proving job-control routes are covered by the
  existing global rate-limit middleware and return `429 RATE_LIMITED` when over
  limit.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobcontrol/service.go
mm-chat/backend/internal/jobcontrol/handler.go
mm-chat/backend/internal/jobcontrol/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/jobcontrol ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5c Real voice/image executors with output storage and provider smoke
G6.5d Code execution sandbox contract before any real executor is enabled
```

Next slice: G6.5c should start with real voice/image executor design or a
storage-only result artifact contract; code execution remains blocked until the
sandbox contract is explicit.

## 2026-07-15 — G6.5d Code Execution Sandbox Contract Completed

Objective: define the hard gate for real code execution before any runtime
executor is enabled. This is a contract-only slice; the server route remains
fail-closed and `codeExecution` capability remains disabled.

Completed scope:

- added `docs/contracts/code-execution-sandbox-contract.md` with the required
  seven-section code-spec structure;
- defined request/response signatures, sandbox boundaries, allowed audit fields,
  validation/error matrix, good/base/bad cases, required tests, and wrong vs
  correct execution flow;
- documented that model-simulated execution is not equivalent to sandboxed code
  execution;
- updated contract index and G6 progress ledgers.

Changed surfaces for this slice:

```text
mm-chat/docs/contracts/code-execution-sandbox-contract.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5c Real voice/image executors with output storage and provider smoke
```

Next slice: either implement storage-first voice/image result artifacts or defer
real provider execution until credentials and smoke target are available.

## 2026-07-15 — G6.5c.1 Storage-only Voice/Image Artifact Boundary Completed

Objective: create the backend storage seam that future real voice/image
executors must use for generated outputs, without enabling any provider call,
credential use, or quota-consuming live smoke.

Completed scope:

- added `internal/jobartifacts`, a small Go service that accepts future job
  result streams and stores them through the existing `files.Service.Upload`
  boundary;
- mapped image results to file purpose `image` and audio results to purpose
  `audio`;
- validated artifact kind, positive declared size, non-nil body, and matching
  `image/*` or `audio/*` content type before upload;
- sanitized display filenames and client job identifiers so executor outputs do
  not pass path fragments into file metadata;
- returned only compact artifact metadata (`fileId`, `purpose`, `contentType`,
  `size`) and kept generated bytes behind backend file/object storage;
- left `voice` and `imageGeneration` capabilities disabled and did not touch any
  real STT/TTS/image provider configuration or quota.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobartifacts/artifacts.go
mm-chat/backend/internal/jobartifacts/artifacts_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/jobartifacts # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                  # passed
git diff --check -- mm-chat                                                         # passed
```

Residual G6 blockers:

```text
G6.5c.2 Real voice executor with stored audio artifacts and configured-provider smoke
G6.5c.3 Real image executor with stored image artifacts and configured-provider smoke
```

Next slice: wire one real executor at a time behind this artifact boundary only
after an explicit live-provider smoke target and quota/credential approval.

## 2026-07-15 — G6.5c.2a Voice Executor Opt-in Seam Completed

Objective: add the Go service seam needed by future voice transcription and
speech-synthesis executors while keeping the default runtime fail-closed and
avoiding any real provider call, credential use, or quota-consuming smoke.

Completed scope:

- added a `voicejobs.Executor` interface with `Transcribe` and `Synthesize`
  methods;
- passed validated multipart audio metadata and stream handles from
  `/v1/voice/transcribe` into the service only after admission validation;
- added `WithExecutor` as the explicit opt-in gate, so the default service still
  returns `VOICE_JOBS_UNAVAILABLE`;
- added a sanitized `admitted` audit status and fail-closed audit gate requiring
  an explicit audit recorder before any configured voice executor can run;
- required an artifact store before any synthesis executor can run, returning
  `VOICE_ARTIFACT_STORE_UNAVAILABLE` before executor invocation when storage is
  absent;
- stored synthesized audio executor output through the G6.5c.1 artifact
  boundary and returned only compact artifact metadata;
- covered the seam with fake in-process executors/stores only. No live STT/TTS
  provider was called.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/voicejobs/types.go
mm-chat/backend/internal/jobaudit/jobaudit.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/service_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/jobaudit ./internal/voicejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3 Real image executor with stored image artifacts and configured-provider smoke
```

Next slice: either implement the image executor opt-in seam or add the real
voice provider behind an explicit quota/credential approval gate.

## 2026-07-15 — G6.5c.3a Image Executor Opt-in Seam Completed

Objective: add the Go service seam needed by future image-generation executors
while keeping the default runtime fail-closed and avoiding any real image
provider call, credential use, or quota-consuming smoke.

Completed scope:

- added an `imagejobs.Executor` interface and explicit `WithExecutor` opt-in
  gate;
- required a configured image artifact store before any image executor can run,
  returning `IMAGE_ARTIFACT_STORE_UNAVAILABLE` before executor invocation when
  storage is absent;
- required an explicitly configured sanitized `admitted` audit recorder before
  executor invocation, so audit absence/failure prevents provider calls;
- stored generated image executor streams through the G6.5c.1 artifact boundary
  as `image` purpose files and returned only compact artifact metadata;
- added `docs/contracts/media-job-executor-seams.md` as the seven-section
  executable contract for voice/image executor gates;
- covered the seam with fake in-process executors/stores only. No live image
  provider was called.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/imagejobs/types.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/service_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3b Real provider-backed image executor and authorized configured-provider smoke
```

Next slice: either add real provider code behind explicit quota/credential
approval or move to the next non-provider G6 hardening slice.

## 2026-07-15 — G6.5e Live Provider Smoke Authorization Gate Completed

Objective: add a reusable default-deny authorization gate for any future live
voice/image provider smoke, so executor seams cannot accidentally consume
supplier quota just because provider credentials exist.

Completed scope:

- added `internal/providersmoke`, a provider-free Go package that authorizes
  live provider smoke only when all required env values are present;
- required `MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=true`, the exact approval text
  `I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA`, a sanitized run id, and an exact
  `kind:providerId:modelId` target match;
- limited live-smoke target kinds to `voice.transcribe`, `voice.synthesize`, and
  `image.generate`;
- made authorization errors wrap a stable `ErrNotAuthorized` and expose only
  codes, not provider/model/prompt/credential values;
- documented the env keys in backend and single-server example env files only;
- added `docs/contracts/provider-live-smoke-authorization.md` as the
  seven-section executable contract for quota-consuming smoke gates.

Changed surfaces for this slice:

```text
mm-chat/backend/.env.example
mm-chat/.env.single-server.example
mm-chat/backend/internal/providersmoke/gate.go
mm-chat/backend/internal/providersmoke/gate_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/providersmoke # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                  # passed
git diff --check -- mm-chat                                                         # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3b Real provider-backed image executor and authorized configured-provider smoke
```

Next slice: wire a real provider only after the owner chooses a provider target
and explicitly authorizes quota-consuming live smoke.

## 2026-07-15 — G6.5c.3b.1 OpenAI-compatible Image Executor Added, Live Smoke Blocked by Current Provider

Objective: after owner approval to use provider quota, add the real
OpenAI-compatible image executor and run an authorized live image smoke without
exposing provider secrets.

Completed scope:

- added `imagejobs.OpenAICompatibleExecutor`, posting to
  `/images/generations` with `model`, `prompt`, `n`, and optional `size`;
- accepted both `b64_json` provider responses and provider-hosted image URLs;
- converted provider image responses into `GeneratedImageResult` streams for
  the existing image artifact boundary;
- added fake-transport tests for request shape, Authorization header use,
  base64 decoding, URL image fetch, unsupported provider IDs, and non-leaky
  provider-status errors;
- added `TestLiveOpenAICompatibleImageGenerationSmoke`, which is skipped by
  default and only runs after the G6.5e live-smoke authorization gate passes;
- attempted live smoke with the owner-approved quota path.

Live smoke evidence:

```text
Normal sandbox smoke:
  result: blocked before provider by local proxy/socket sandbox

Escalated configured relay smoke:
  endpoint class: configured OpenAI-compatible relay
  result: provider reached, /v1/images/generations returned HTTP 404

Escalated direct OpenAI smoke with the same configured key:
  endpoint class: https://api.openai.com/v1
  result: provider reached, returned HTTP 401
```

No provider key, prompt body, response body, or `.env.single-server` content was
printed. No image artifact was produced because no image-capable endpoint/key
completed successfully.

Changed surfaces for this slice:

```text
mm-chat/.env.single-server.example
mm-chat/backend/.env.example
mm-chat/backend/internal/imagejobs/openai_compatible_executor.go
mm-chat/backend/internal/imagejobs/openai_compatible_executor_test.go
mm-chat/backend/internal/imagejobs/openai_compatible_live_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.3b.2 Authorized configured-provider image smoke passes against an image-capable key/endpoint
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
```

Next slice: provide or configure an image-capable endpoint/key, or add a
provider-specific executor such as Agnes/Gemini if that is the intended image
supplier.

## 2026-07-15 — G6.5c.3b.2 Authorized OpenAI-Compatible Image Smoke Passed

Objective: verify the real OpenAI-compatible image executor against an
owner-approved image-capable provider without persisting or printing provider
credentials.

Official API contract checked:

- direct image generation uses `POST /v1/images/generations`;
- the request body is the OpenAI Images API shape: `model`, `prompt`, optional
  `n`, and optional `size`;
- `gpt-image-2` is an OpenAI image model target;
- GPT image generation responses provide generated image bytes via
  `data[].b64_json` by default.

Live smoke evidence:

```text
Configured endpoint class: OpenAI-compatible relay
Target: image.generate:openai:gpt-image-2
Result: passed
Stored artifact: /tmp/mm-chat-provider-smoke/1-generated-1.png
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs -run TestLiveOpenAICompatibleImageGenerationSmoke -count=1 -v # passed
```

No provider key, `.env.single-server` content, prompt body beyond the existing
smoke-test prompt, provider response body, or generated image bytes were added
to the repository. The generated image artifact remains in `/tmp` only.

Completed G6 image-executor blockers:

```text
G6.5c.3b.2 Authorized configured-provider image smoke passes against an image-capable key/endpoint
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
Route-wiring/capability-reopen slice for imageGeneration remains separate from this smoke.
```

## 2026-07-15 — G6.5c.3c Go Image Route Wired to Executor/Storage/Audit

Objective: connect the already verified OpenAI-compatible image executor to the
real Go HTTP route without enabling any browser-side fallback or exposing
provider secrets.

Completed scope:

- added `httpserver.WithImageJobService` so `/v1/images/generations` can be
  wired to a configured service instead of the default fail-closed handler;
- updated `cmd/api` to build an image job service with:
  - sanitized structured `job_audit` events;
  - OpenAI-compatible executor opt-in when `PROVIDER_TYPE` is OpenAI-compatible
    and server-only `PROVIDER_BASE_URL` plus `PROVIDER_API_KEY` are present;
  - `jobartifacts` storage through the existing backend `files.Service` when
    file repository and object store dependencies are both present;
- preserved fail-closed behavior when provider credentials, file metadata DB,
  or object storage are absent;
- mapped provider-side image failures to `502 IMAGE_PROVIDER_ERROR` without
  leaking prompts, provider bodies, or credentials.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/imagejobs/openai_compatible_executor.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs ./internal/httpserver ./cmd/api # passed
git diff --check -- mm-chat                                                                                         # passed
```

Residual blockers:

```text
Frontend imageGeneration adapter/capability reopen remains separate.
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke remains open.
```

## 2026-07-15 — G6.5c.3d Frontend Server-Mode Image Adapter and Capability Reopen

Objective: reopen the server-mode image generation UI path only after the Go
route, real executor, artifact storage, and live smoke gates were proven.

Completed scope:

- added local/server `ImageGenerationApi` adapters under the frontend API-client
  boundary;
- enabled `imageGeneration` only when `createNeoChatApiClient()` is configured
  for server mode, while keeping `voice` and `codeExecution` disabled;
- changed `generateImage()` so server mode posts to Go
  `/v1/images/generations` instead of falling through to
  `/api/chat/generate-image`;
- mapped returned image artifact metadata to server-backed image attachments
  with bytes fetched through `/v1/files/{fileId}/content`;
- kept local mode on the transitional Next `/api/chat/generate-image` route.

Changed surfaces:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/imageApi.ts
mm-chat/frontend/src/services/api/client/server/imageApi.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/frontend/src/__tests__/skillInvocationWiring.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/byokServices.test.ts # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/byokServices.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/fileService.test.ts src/__tests__/skillInvocationWiring.test.ts # passed
cd mm-chat/frontend && corepack pnpm test # passed with sandbox escalation; ordinary sandbox blocks byok script child process with EPERM
```

Residual blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke remains open.
Search/RAG, code execution, final local-mode removal, and clean-copy deletion gates remain separate slices.
```

## 2026-07-15 — G6.5c.2b.1 OpenAI-Compatible Voice Executor, Route Wiring, and Smoke Harness

Objective: add a real voice-provider executor behind the existing Go voice job
admission/storage/audit seam without reopening frontend voice controls or
requiring live provider quota during normal tests.

Completed scope:

- added `voicejobs.OpenAICompatibleExecutor` for OpenAI-compatible voice APIs:
  - STT: multipart `POST /audio/transcriptions` with `file`, `model`, and
    optional provider language;
  - TTS: JSON `POST /audio/speech` with `model`, `input`, and `voice`;
- mapped provider non-2xx responses to sanitized `502 VOICE_PROVIDER_ERROR`
  without echoing provider bodies, synthesis text, audio bytes, or credentials;
- wired `cmd/api` to construct a voice job service from server-only
  `PROVIDER_BASE_URL` and `PROVIDER_API_KEY` when configured;
- required backend artifact storage before TTS executor calls, preserving the
  existing no-storage fail-closed behavior before quota consumption;
- added `httpserver.WithVoiceJobService` and route-level coverage for configured
  voice synthesis artifacts;
- added a gated live voice smoke harness under the existing
  `providersmoke` authorization contract. Normal test runs skip it unless the
  exact quota approval, run id, and voice target are configured.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/voicejobs/openai_compatible_executor.go
mm-chat/backend/internal/voicejobs/openai_compatible_executor_test.go
mm-chat/backend/internal/voicejobs/openai_compatible_live_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/voicejobs ./internal/httpserver ./cmd/api # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./... # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/voicejobs -run TestLiveOpenAICompatibleVoiceSmoke -count=1 -v # skipped: provider live smoke disabled
```

Residual blockers:

```text
G6.5c.2b.2 Authorized configured-provider voice smoke remains open.
Frontend voice capability/adapter reopen remains separate; `voice` stays disabled in server mode.
```

## 2026-07-15 — G4.5c.2d Plugin Audit Metadata Beyond Installing-User Persistence

Owner direction before this slice: skip the voice real-provider smoke/key path
for now. `G6.5c.2b.2` remains open/deferred; work resumed on the smallest
remaining plugin finalization item.

Objective: add server-side plugin install/execute audit metadata without
recording secrets, argument values, plugin responses, or full outbound URLs.

Completed scope:

- added a Go plugin audit seam with sanitized `plugin.install` and
  `plugin.execute` admission events;
- recorded only bounded metadata: action/status, actor user id, plugin id,
  function name/count, call id, install/execute source, built-in/auth presence,
  argument count, request id/user-agent/IP when available, and host-only URL
  metadata;
- wired plugin install audit before registry mutation and plugin execute audit
  before outbound plugin HTTP execution;
- mapped configured audit sink failures to `503 PLUGIN_AUDIT_UNAVAILABLE`,
  fail-closing before registry writes or outbound plugin calls;
- added a Postgres audit recorder that writes configured-server events to
  `audit_logs` without a new migration; local/dev without `DATABASE_URL` keeps
  the existing memory registry behavior without a mandatory audit sink;
- wired `cmd/api` and `httpserver` so configured Postgres deployments use the
  new plugin audit recorder.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/plugins/audit.go
mm-chat/backend/internal/plugins/audit_postgres.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/plugins/repository_postgres_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/plugins ./internal/httpserver ./cmd/api # passed
```

Residual blockers:

```text
G4.6b Live browser/provider smoke remains open.
G6.5c.2b.2 Authorized configured-provider voice smoke remains deferred/skipped for now.
G9 still owns final removal of local-only transitional Next plugin routes.
```

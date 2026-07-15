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

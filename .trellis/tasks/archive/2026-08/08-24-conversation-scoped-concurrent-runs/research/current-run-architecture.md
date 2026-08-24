# Current Run Architecture and Concurrency Options

## Scope inspected

- Frontend generation controller, Chat orchestration, sidebar, server read
  store, in-memory conversation cache, stream client, CRUD mappings, and tests.
- Backend chat handler, active-run registry, resumable SSE stream, conversation
  list DTO, PostgreSQL conversation/message queries, cancellation, and current
  message indexes.

## Findings

### Backend

- Active runs are registered independently by `runId`; the registry does not
  impose a user-wide, workspace-wide, or conversation-wide singleton.
- Chat generation uses `context.WithoutCancel(request.Context())`, so closing
  or navigating away from the original SSE delivery does not cancel the Run.
- A Run can be cancelled by exact Run ID and its recent stream can be resumed
  by exact Run ID and cursor.
- Each assistant message persists `pending`/`streaming`/terminal status and a
  metadata `runId`, so PostgreSQL can project a conversation's active Run
  without a schema migration.
- The conversation list currently omits that projection, so a refreshed
  browser cannot know which sidebar rows are still running.

### Frontend

- `useChatGenerationController` owns one boolean, one controller, and one Run
  counter for the entire application.
- `ChatApp` uses that global boolean to disable the composer and guard send,
  regenerate, edit, permission, delete, and stop behavior.
- `chatStore` has one `ServerGenerationState` and one global
  `serverReadRequestId`. Selecting another conversation intentionally
  invalidates callbacks from the previous stream.
- Existing tests explicitly preserve the old behavior where navigation makes
  a stream stale; those tests must be replaced by conversation-scoped
  concurrency and background-update assertions.
- The server conversation cache is already keyed by conversation and can hold
  background message trees, but streaming currently deletes or ignores it.

## Feasible approaches

### A. Browser-only active-session set

- Replace the global boolean/controller with maps keyed by conversation ID.
- Add a spinner from those maps.
- Pros: smallest patch; no Backend changes.
- Cons: refresh loses truth; another tab is invisible; a detached Run can leave
  a false idle row until the user opens the conversation.

### B. Conversation-scoped browser registry plus server active-Run projection

- Add an optional active-generation summary to the conversation list DTO,
  derived from the current `messages` table.
- Key controllers, stream request tokens, generation state, background cache
  updates, composer locks, stop, and delete by conversation ID.
- Poll/reconcile only while active rows exist so refreshed or detached Runs
  eventually reach their terminal state.
- Pros: truthful sidebar, no migration, supports same-workspace parallelism,
  preserves exact cancel authority, and fits existing detached SSE semantics.
- Cons: touches Frontend and Backend contracts and requires careful stale-event
  tests.

### C. General durable Run collection/subscription API

- Add user-level Run listing and multiplexed subscriptions, then model the UI
  entirely from that control plane.
- Pros: strongest basis for a future task center and cross-device monitoring.
- Cons: much larger API/storage surface than the current sidebar need.

## Recommendation

Use Approach B. Keep a strict one-active-Run-per-conversation client invariant,
but allow any number of different conversations—including siblings in one
workspace—to run concurrently. Reserve Approach C for a future task center.

## Relevant specs

- `.trellis/spec/frontend/state-management.md`
- `.trellis/spec/frontend/host-workspaces.md`
- `.trellis/spec/frontend/agent-transcript.md`
- `.trellis/spec/backend/chat-tool-loop.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

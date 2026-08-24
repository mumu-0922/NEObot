# Conversation-Scoped Concurrent Runs

## Goal

Let users leave a running Chat/Agent conversation, see that run continue from
the sidebar, and start an independent run in another conversation—including a
second conversation inside the same Host Workspace.

## What I already know

- The unified sidebar now renders conversations directly beneath their Host
  Workspace or the temporary-conversation section.
- The sidebar currently exposes no per-conversation running indicator.
- `ChatApp` receives one `isGenerating` value from
  `useChatGenerationController`; many send/edit/regenerate/permission paths
  treat it as a global lock.
- `useChatGenerationController` owns one `AbortController`, which strongly
  suggests the browser currently models only one active foreground run.
- The Backend exposes durable generation statuses including `pending`,
  `streaming`, `completed`, `failed`, and `cancelled`.
- Backend Runs are keyed by `runId`; its active-run registry has no global user
  or workspace singleton, and generation deliberately survives SSE delivery
  disconnects through `context.WithoutCancel`.
- PostgreSQL already stores active assistant status and metadata `runId`; the
  conversation list simply does not expose that projection yet.

## Confirmed Product Decisions

- Running state should belong to a conversation/run, not to a workspace.
- Parallelism should be allowed across different conversations, including
  conversations in the same workspace.
- A single conversation should still permit only one active generation at a
  time to prevent message-order and revision conflicts.
- Switching conversations must not abort the run being left.
- Copy Pi Web's complete background-session UX: show an animated running
  indicator while active, then mark a background conversation unread when its
  Run finishes.

## Requirements (evolving)

- Show an accessible animated running indicator beside every conversation with
  an active `pending` or `streaming` run.
- Preserve the indicator while another conversation is selected.
- Allow one active run per conversation and multiple active runs across
  conversations, regardless of shared workspace membership.
- Stop/cancel must target only the selected conversation's active run.
- Completion/failure/cancellation must clear the correct conversation state
  without affecting other runs.
- A Run that finishes while its conversation is not selected must leave a
  visible unread marker until that conversation is opened.
- Reload and reconnect behavior must derive running state from server-owned
  generation authority where supported.
- The server conversation list must expose an optional active-generation
  summary derived from existing message rows; no schema migration is needed.
- Reconcile active rows while they exist so detached or refreshed runs clear
  without requiring the user to open each conversation.

## Acceptance Criteria (evolving)

- [x] Start a run in conversation A, switch to B, and observe a spinner on A.
- [x] Start a run in B while A is still running; both sidebar rows show a
      spinner and both complete independently.
- [x] A and B may belong to the same workspace.
- [x] Sending a second turn in the same conversation while it is active remains
      blocked or explicitly queued; it must not corrupt ordering.
- [x] Stopping B does not cancel A.
- [x] Switching away and back does not abort, duplicate, or lose streamed and
      durable events.
- [x] Reload/reconnect produces a truthful state without a permanently stuck
      spinner.
- [x] A background Run that finishes changes from a spinner to an unread
      marker; selecting the conversation clears the marker.
- [x] Keyboard and screen-reader users receive a non-noisy running-state label.

## Definition of Done

- Focused Frontend and Backend tests cover concurrency, cancellation,
  switching, reload/replay, and the sidebar projection.
- Frontend format, lint, typecheck, tests, and production build pass.
- Backend tests run if the server contract changes.
- Runtime rollout is performed with a retained Frontend rollback image and
  live acceptance evidence.
- Specs and user-facing behavior notes are updated.

## Out of Scope (provisional)

- More than one simultaneous run inside the same conversation.
- A general-purpose task queue, scheduler, or cross-user admin dashboard.
- Arbitrary cancellation of background conversations directly from the
  sidebar unless existing Backend semantics make it trivial and safe.

## Technical Notes

- Initial search found the global generation controller at
  `mm-chat/frontend/src/features/chat/hooks/useChatGenerationController.ts`.
- Main orchestration and global guards are in
  `mm-chat/frontend/src/components/app/ChatApp.tsx`.
- Sidebar projection lives in
  `mm-chat/frontend/src/components/layout/Sidebar.tsx`.
- Durable generation mapping lives in
  `mm-chat/frontend/src/store/core/chatStore.ts` and typed server API clients.
- Research and feasible approaches are recorded in
  [`research/current-run-architecture.md`](research/current-run-architecture.md).
- Pi Web's session ownership, per-session admission, sidebar running-set, and
  reconnect behavior are recorded in
  [`research/pi-session-concurrency.md`](research/pi-session-concurrency.md).

## Verification and rollout

- `bash scripts/verify-standalone.sh --full` passed against the final source:
  Frontend 197 files / 978 tests, all Backend packages, and RAG 1,910 passed /
  7 skipped.
- Focused Backend and Frontend tests cover per-Conversation admission, sibling
  concurrency, user isolation, exact cancellation, background completion,
  unread clearing, reconciliation, and accessible sidebar indicators.
- Source-built Backend and Frontend images were deployed as
  `pi-conversation-runs-20260824T040700Z`; both services and the proxied runtime
  health endpoints returned healthy/HTTP 200. The prior image IDs remain
  available locally as rollback points.

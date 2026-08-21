# Preserve Downstream Messages During Regeneration

## Goal

Complete the answer-slot isolation contract at the regeneration entry point so
regenerating an earlier Assistant answer never removes the already visible
messages below it.

## Requirements

- When Local regeneration creates a new sibling Assistant placeholder, move
  the source answer's descendant attachment to the new active sibling before
  rendering it.
- When Server regeneration receives the new `message.started` draft, create it
  through the same descendant-preserving sibling operation rather than the
  generic parent append path.
- Keep every node and inactive subtree; do not delete or duplicate descendants.
- Repair direct child node parent links after moving the continuation.
- Keep ordinary new-message append, edited User branches, and subsequent
  answer-version switching behavior unchanged.
- Build and deploy only the Frontend, using focused tests and automatic commits
  without Push.

## Acceptance Criteria

- [x] Local `addMessageVersion` immediately renders the new empty Assistant
      version followed by the exact same downstream message IDs and order.
- [x] Server regeneration `message.started` immediately renders the new
      Assistant draft followed by the exact same downstream message IDs/order.
- [x] Completing either regeneration updates the new answer without disturbing
      its continuation.
- [x] Switching between the old and regenerated answers remains slot-local.
- [x] All tree nodes remain reachable exactly once and direct child node parent
      IDs match the selected answer.
- [x] Focused tree/store tests, changed-file format/lint, typecheck, production
      build, Frontend rollout, and live health checks pass.

## Technical Approach

Change `createModelResponseBranch` so the new model sibling inherits the source
model node's child list and active child, the source becomes childless, and
each moved direct child points at the new sibling. Reuse this helper in Local
version creation and in the Server regeneration-only draft insertion path.
Keep generic server message insertion unchanged for normal messages.

## Decision (ADR-lite)

**Context**: The prior fix preserved descendants only when navigating existing
versions. Regeneration first activated a brand-new childless sibling, so the
continuation disappeared before navigation could help.

**Decision**: Preserve the active continuation at sibling creation time, then
use the existing descendant-swap operation for later version navigation.

**Consequences**: Regeneration and navigation now share one answer-slot model;
no API, backend, database, or persisted-schema migration is needed.

## Out of Scope

- Backend generation behavior or provider requests.
- User-edit branch semantics.
- Visual restyling and unrelated tests.

## Technical Notes

- Pure tree helper: `mm-chat/frontend/src/lib/chat/messageTree.ts`.
- Local/Server orchestration: `mm-chat/frontend/src/store/core/chatStore.ts`.
- Regression suites: `messageTree.test.ts`, `chatStore.test.ts`, and
  `chatStoreServerRead.test.ts`.
- This task closes an incomplete prior fix demonstrated by the user's live
  screenshot after regeneration.

## Debug Retrospective

- Root cause category: change-propagation failure plus a test-coverage gap.
- The prior patch enforced slot-local behavior only in the navigation actions;
  it missed both sibling-creation entry points: Local `addMessageVersion` and
  Server regeneration draft insertion.
- Prevention is now structural: `createModelResponseBranch` owns descendant
  transfer, both modes reuse it, and the Server test asserts state immediately
  inside `message.started`, not only after the terminal result.
- The executable contract is recorded in
  `.trellis/spec/frontend/state-management.md` so future changes must cover
  creation and selection together.

## Rollout Evidence

- Work commit: `b87425ed`.
- Frontend image: `mm-chat/frontend:regen-downstream-b87425ed-20260821T094432Z`
  (`sha256:a87be57fcbb23e0a3528c14d0a8427a209fa8860966cca2409a1407e8f338afc`).
- Rollback snapshot:
  `mm-chat/backup/deployments/20260821T094432Z-regen-downstream-b87425ed/`.
- Focused verification passed: 76 tree/Local-store/Server-store Vitest tests,
  changed-file Prettier and ESLint, TypeScript typecheck, and host/Docker
  production builds. No unrelated full suite or standalone gate was run.
- Live root and `/api/health` returned HTTP `200`; the new Frontend and unchanged
  Backend containers are healthy.
- Roll back by restoring the snapshot's `.env.single-server.before` and
  recreating only `frontend`; no schema or durable-state rollback is required.

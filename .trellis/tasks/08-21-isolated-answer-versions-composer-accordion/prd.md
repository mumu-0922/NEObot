# Isolated Answer Versions and Composer Accordion Menus

## Goal

Make assistant answer-version navigation replace only the selected answer while
keeping the already visible downstream conversation stable, and make the four
composer configuration menus behave as a single-open accordion group.

## Requirements

- Switching an assistant answer between `1/N`, `2/N`, and other versions must
  not add, remove, reorder, or reposition any downstream visible message.
- The selected assistant version must become the actual active tree node so
  later message operations continue from the answer the user sees.
- Preserve all inactive tree nodes and descendants; answer switching must not
  delete historical branches.
- User-message edit branch switching keeps its existing branch semantics,
  because selecting another user prompt is expected to select its matching
  continuation.
- Apply the assistant-version behavior consistently in Local and Server modes.
- `Conversation mode`, `Agent permission`, `Reasoning effort`, and `Web search`
  menus share one controlled open state. Opening one closes whichever of the
  other three was open.
- Existing option-selection, full-access confirmation, disabled states,
  keyboard behavior, and menu dismissal remain unchanged.

## Acceptance Criteria

- [ ] Given an active assistant version with downstream messages, switching to
      a sibling version changes only that assistant message; downstream message
      IDs and order stay identical.
- [ ] Repeated next/previous switching preserves the currently visible
      downstream path and does not lose inactive subtrees.
- [ ] User-message branch switching still selects that user's own continuation.
- [ ] Local store switching persists the updated tree and keeps the visible
      message count stable.
- [ ] Server store switching updates only Server read state/cache and does not
      write Local IndexedDB state.
- [ ] Opening any one of the four composer configuration menus closes the
      previously open configuration menu.
- [ ] Focused Vitest coverage, changed-file formatting/lint, TypeScript
      typecheck, and production frontend build pass.
- [ ] Updated frontend image is deployed and the application health check
      passes.

## Definition of Done

- Focused regression tests cover the pure tree contract, Local/Server store
  paths, and composer controlled-open composition.
- No backend/API/schema changes are introduced.
- Only the frontend is rebuilt and rolled out unless inspection proves another
  component is required.
- Work, rollout record, task archive, and journal are committed automatically;
  nothing is pushed.

## Technical Approach

Add a message-tree operation for version switching that swaps the two model
nodes' descendant attachments before activating the target sibling. This keeps
the currently rendered continuation attached to the selected answer without
sharing child nodes or breaking the tree's single-parent invariant. Keep the
existing branch switch operation for user edits. Route both store version
actions through the role-aware operation.

In `MessageInput`, use one local discriminated union for the four configuration
menu IDs and control each Radix `DropdownMenu.Root` through `open` and
`onOpenChange`. Do not introduce four independent booleans.

## Decision (ADR-lite)

**Context**: The current tree encodes answer variants and conversation branches
with the same parent/child edges. Activating a sibling answer therefore also
changes which continuation is reachable.

**Decision**: For model sibling navigation, move the currently visible
descendant attachment by swapping descendant lists between current and target
nodes, update direct children's parent IDs, then activate the target. This
keeps the target node authoritative for later actions while preserving every
subtree. User branch navigation remains unchanged.

**Consequences**: Answer navigation becomes slot-local as requested. Historical
inactive continuations remain stored, but their attachment follows the inactive
answer when required to preserve the currently visible layout; no persistence
or API migration is needed.

## Out of Scope

- Changing regeneration-time behavior before a new answer completes.
- Redesigning the full message-tree persistence schema or backend API.
- Making attachment/model selector menus part of the four-item configuration
  accordion.
- Visual restyling or translation changes.
- Full repository or full standalone test suites.

## Technical Notes

- Tree logic: `mm-chat/frontend/src/lib/chat/messageTree.ts`.
- Store paths: `switchServerMessageVersion` and `switchMessageVersion` in
  `mm-chat/frontend/src/store/core/chatStore.ts`.
- UI menus: `mm-chat/frontend/src/components/chat/MessageInput.tsx`.
- Tests: `messageTree.test.ts`, `chatStore.test.ts`,
  `chatStoreServerRead.test.ts`, and `messageInputComposition.test.ts`.
- User explicitly confirmed the UX through screenshots and requested direct
  implementation, focused tests only, automatic commit/build/deploy, and no
  confirmation prompts.

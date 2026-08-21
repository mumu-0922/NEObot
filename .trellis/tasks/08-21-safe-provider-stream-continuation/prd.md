# Safe Provider Stream Continuation

## Goal

Let a user safely continue a partially rendered assistant answer after an exact
`PROVIDER_STREAM_INTERRUPTED` failure without replaying any Tool, Search,
Knowledge, Memory, Goal, Skill, MCP, File, Terminal, Browser, or Host Workspace
side effect that already ran in the failed Turn.

## Requirements

- Add a first-class `Continue answer` action only for server messages with a
  non-empty partial answer and the exact recoverable
  `PROVIDER_STREAM_INTERRUPTED` code.
- Keep ordinary `Regenerate` available as the explicit full rerun path.
- A continuation creates a new sibling assistant version. The failed source
  message and its durable Agent events remain immutable and inspectable.
- The Backend, not client configuration, authorizes continuation from the
  exact owned Conversation and source message. The source must be an Assistant
  message, be `failed`, have the exact interruption code, contain partial text,
  and belong to the submitted parent User message.
- The continuation Provider request includes the exact preserved partial answer
  and bounded, sanitized, explicitly untrusted durable presentation evidence.
  It asks for only the missing answer suffix.
- Physically omit every Tool runtime and both built-in and external Search from
  the continuation request. Do not perform new Knowledge, Memory, Goal, Skill,
  MCP, local/Host, Search, or direct-memory planning work.
- Deny continuation when the durable source Turn contains an unresolved,
  approval-waiting, running, interrupted, or outcome-unknown Tool state.
- Stream the preserved prefix immediately, append the Provider suffix, persist
  the combined answer in the new sibling, and retain another interruption as a
  recoverable partial result that can be continued again.
- Preserve the existing reconnect/cursor behavior, cancellation, runtime
  Provider selection, Conversation permission authority, Agent transcript,
  and legacy Regenerate behavior.
- Use a request idempotency key and prevent UI double submission through the
  existing active-generation guard.
- Add Chinese, English, and Japanese copy explaining that Continue does not
  rerun completed Tools.
- Automatically commit, build immutable Backend/Frontend images, update the
  active single-server deployment, verify health, and do not Push.

## Acceptance Criteria

- [ ] An eligible interrupted partial answer shows `Continue answer` and
      retains `Regenerate` as a separate full-rerun action.
- [ ] Continue creates a sibling assistant version whose initial content is the
      exact preserved prefix and whose completed content is prefix plus suffix.
- [ ] Backend-focused tests prove the continuation request reaches only the
      plain Provider stream path and includes no callable Tools or Search path.
- [ ] Backend-focused tests reject wrong role/status/error/parent, empty
      partial content, cross-Conversation lookup, and unresolved Tool state.
- [ ] A second Provider interruption preserves the longer partial result and
      remains eligible for another continuation.
- [ ] The source message, source Agent events, and completed Tool call IDs are
      unchanged; no Tool execution event is copied or rerun.
- [ ] Frontend focused tests cover request wiring, eligibility, visible copy,
      active-generation locking, and coexistence with Regenerate.
- [ ] Focused Backend tests/race/vet and Frontend format/lint/typecheck/tests/
      production build pass. Full standalone verification is not run unless a
      discovered cross-domain risk makes it necessary.
- [ ] Immutable images are deployed with a backup/rollback point and the live
      Backend, Frontend, and PostgreSQL services are healthy.

## Definition of Done

- Code, focused tests, localization, API contract notes, rollout/rollback
  notes, and relevant Trellis specs are updated.
- The feature is exercised through one controlled interrupted-stream fixture
  and one live web canary when available.
- Changes are committed automatically in focused commits and are not pushed.

## Technical Approach

Extend the existing `/v1/chat/conversations/{id}/stream` request with an
optional `continuationOfMessageId`. The handler validates the durable source
message and constructs a continuation-only Provider history ending with the
failed Assistant prefix plus a synthetic, bounded suffix-only instruction.
Continuation mode bypasses every Tool/Search/RAG/Memory preparation branch and
calls `Provider.StreamChat` directly. A new sibling Assistant message stores
`continuationOfMessageId` metadata and the combined prefix/suffix content.

The frontend reuses the proven server regeneration stream machinery with a
continuation option, so live events, reconnect, branch projection, and terminal
state handling stay on one path. `MessageItem` adds a prominent error-card
action while the existing Regenerate toolbar action remains unchanged.

## Decision (ADR-lite)

**Context:** Replaying a failed Agent Turn can duplicate filesystem writes,
commands, commits, remote MCP writes, or other side effects even though only the
final Provider answer stream failed.

**Decision:** Continue through a new immutable sibling answer and a Backend-
enforced answer-only Provider request. Never reopen or mutate the failed Turn,
never copy its Tool events, and never trust the browser to disable Tools.

**Consequences:** The source transcript remains the authority for what ran. The
new version can safely finish the prose but cannot obtain new evidence; users
must choose Regenerate when they intentionally want a fresh execution.

## Out of Scope

- Resuming the original closed SSE/TCP connection byte-for-byte.
- Automatically retrying or replaying any Tool, including nominally read-only
  Tools.
- Mutating a failed Assistant row or appending new events to its ended Turn.
- Recovering unsanitized ephemeral Tool results that were intentionally never
  persisted.
- Automatically continuing without an explicit user action.

## Technical Notes

- Existing failure mapping: `mm-chat/backend/internal/chat/handler.go`.
- Existing durable authority: `chat_agent_events` and
  `mm-chat/backend/internal/chat/chat_agent_events.go`.
- Existing server branch regeneration:
  `mm-chat/frontend/src/store/core/chatStore.ts` and
  `mm-chat/frontend/src/components/app/ChatApp.tsx`.
- Existing error rendering:
  `mm-chat/frontend/src/components/chat/MessageItem.tsx`.
- Research reference:
  [`research/current-continuation-seams.md`](research/current-continuation-seams.md).


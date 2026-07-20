# G11.13 Conversation Context and Memory Plan

Status: active. G11.13A is complete; later slices remain deliberately separate.

## Decision

Server chat context is reconstructed from Postgres on every answer request. The
browser selects and persists the branch parent, but it is not the history
authority. The implementation combines Cherry Studio's current-branch replay
with Nanobot-style token-driven soft consolidation in a later slice.

Do not use idle-time deletion or a browser-only context cache. A cache may
accelerate reads later, but it must never replace persisted messages.

## Slice sequence

### [x] G11.13A — Current branch replay

- Add a provider-neutral ordered `ProviderMessage` contract.
- Reconstruct the effective root-to-current-user path using, in order:
  `metadata.treeParentMessageId`, persisted `parent_message_id`, then the legacy
  active-path fallback for old rows with neither field.
- Treat explicit `treeParentMessageId: null` as a new root.
- Send only the selected sibling branch to Chat Completions and OpenAI
  Responses built-in Search.
- Replace only the current user item's content with the final RAG/Web-enhanced
  prompt; keep older user/assistant content unchanged.
- Persist the current active leaf as `parentMessageId` for future ordinary
  browser sends.
- Preserve the old single-prompt Provider fallback for title, related-question,
  rewrite, tool-planning, and test callers.

Acceptance:

- linear, sibling, legacy-null-parent, explicit-root, and grounded-current-
  prompt tests pass;
- both provider payload families contain ordered history;
- frontend first-message behavior remains parentless while follow-ups persist
  the active leaf;
- a real two-turn provider replay still recalls an unpredictable marker after
  a Go backend restart;
- the disposable conversation and local proof artifacts are deleted.

### [ ] G11.13B — Token-budget soft consolidation

- Add model-aware input budgeting before Provider dispatch.
- Keep a bounded recent raw-message tail and replace older turns with a
  versioned rolling summary when the high-water mark is crossed.
- Persist summary source boundaries and generation metadata in Postgres.
- Never delete the original message rows; rebuilding or changing models must
  remain possible.
- Fail safely to a smaller deterministic history window if summary generation
  is unavailable.

### [ ] G11.13C — Optional durable user memory

- Extract stable preferences/facts separately from conversation summaries.
- Require explicit user controls to inspect, edit, disable, and delete memory.
- Retrieve only relevant memory for a request; never inject the entire memory
  store.
- Keep memory optional: ordinary same-conversation continuity must not depend
  on it.

## Deferred boundaries

- Historical image bytes are not rehydrated in G11.13A; the current user image
  path remains supported. Historical multimodal replay needs its own bounded
  storage/provider compatibility slice.
- Parent ownership validation and single-message child reparenting are separate
  repository-hardening work, not hidden inside context replay.
- No provider response cache is introduced. Caching nondeterministic answers
  would be a product choice, not a memory fix.

## Rollback

Revert the G11.13A commit. New `parent_message_id` values are compatible with
the previous schema and UI, so rollback requires no data migration. Existing
messages remain authoritative in Postgres.

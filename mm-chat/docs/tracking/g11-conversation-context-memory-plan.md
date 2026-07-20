# G11.13 Conversation Context and Memory Plan

Status: active. G11.13A and G11.13B are complete; durable user memory remains a
deliberately separate slice.

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

### [x] G11.13B — Token-budget soft consolidation

- Apply a server-owned conservative context-window registry: 32K fallback,
  16K for `gpt-3.5*`, and 128K for the supported `gpt-4o*`, `gpt-4.1*`,
  `gpt-5*`, `o1*`, `o3*`, and `o4*` families. Browser request fields cannot
  raise these limits.
- Reserve 8,192 output tokens and a 2,048-token estimation safety buffer,
  trigger consolidation at 80% of the remaining input budget, and target 50%.
- Estimate ASCII conservatively at four characters per token, non-ASCII at two
  tokens per rune, message framing explicitly, and each current image at a
  fixed 1,024-token safety allowance.
- Keep a user-boundary recent raw-message tail and replace only the older prefix
  with one server-generated assistant summary plus a server-owned instruction
  that treats summary content as lower-priority untrusted history.
- Persist one versioned active summary per conversation in
  `conversation_context_summaries`, including first/last source message IDs,
  source count, SHA-256 prefix digest, generation model, and token estimates.
- Reuse a summary only when its exact ID/role/content prefix digest matches the
  current branch. Editing or switching a summarized prefix invalidates it.
- Roll a valid summary forward by supplying the previous summary plus only the
  newly evicted messages to the summarizer.
- Never delete or rewrite original messages. Summary generation/read/write,
  oversize-output, or unsafe-boundary failure falls back to a deterministic
  recent tail and records only a bounded degradation code in assistant metadata.

Acceptance:

- budget, multilingual estimation, user-boundary selection, branch digest,
  restart reuse, rolling version, Handler metadata, failure fallback, Service
  validation, migration, least-privilege Repository, and cross-conversation
  rejection tests pass;
- a disposable Postgres database applies migration `034`, round-trips and
  increments a summary, rejects a foreign boundary, and is deleted;
- the live stack applies schema `034`; a real long `gpt-5.6-sol` conversation
  creates summary v1, recalls a marker found only in its summarized prefix,
  restarts the backend, reuses v1 without regeneration, recalls the marker
  again, and hard-deletes all fixture conversation/message/summary rows.

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

For G11.13A, revert its commit; new `parent_message_id` values remain backward
compatible. For G11.13B, roll the backend back first. The old backend safely
ignores `conversation_context_summaries`, so leaving migration `034` applied is
the lowest-risk rollback. After confirming no newer backend is running, the
matching down migration may remove only derived summaries. Original messages
remain authoritative and require no data rollback.

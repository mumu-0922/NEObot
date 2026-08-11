# Memory v2 governance contracts

## 1. Scope / Trigger

Apply this contract when changing migrations `060_memory_governance_ui` or
`072_memory_review_stale_reject`, the authenticated Project or Conversation
Memory policy APIs, scoped Memory CRUD, Review decisions, governance detail/
snapshot hydration, the assistant Activity chip, or the v1 Global Memory
repository after migration `060`.

PR9 is a governance surface, not a reader promotion. The v1 Global Top 5
remains the only prompt and Usage authority. Project/Conversation Memory is
visible to governance APIs only, except that an effective Conversation
`Use Memory = off` prevents the existing v1 prompt injection.

## 2. Signatures

HTTP boundaries:

```text
GET    /v1/memory-governance
POST   /v1/memory-governance/memories
PATCH  /v1/memory-governance/memories/{memoryId}
DELETE /v1/memory-governance/memories/{memoryId}
GET    /v1/memory-governance/memories/{memoryId}/details

GET    /v1/projects
POST   /v1/projects
PATCH  /v1/projects/{projectId}

GET    /v1/chat/conversations/{conversationId}/memory-policy
PATCH  /v1/chat/conversations/{conversationId}/memory-policy

GET    /v1/memory-reviews
POST   /v1/memory-reviews/{suggestionId}/decision
GET    /v1/memory-activities?assistantMessageId={id}&limit=1..20
POST   /v1/memory-activities/{activityId}/undo
```

Migration `060` grants `go_api_runtime` only these new SQL capabilities:

```text
memory_governance_snapshot(UUID)
memory_governance_create_project(UUID, UUID, TEXT, TEXT)
memory_governance_update_project(UUID, UUID, BIGINT, TEXT, TEXT, TEXT)
memory_governance_get_conversation_policy(UUID, UUID)
memory_governance_update_conversation_policy(UUID, UUID, BIGINT, UUID, TEXT, TEXT)
memory_governance_upsert_global_legacy(UUID, UUID, TEXT, TEXT, TEXT,
  SMALLINT, TEXT[], UUID, UUID, BOOLEAN)
memory_governance_update_global_legacy(UUID, UUID, TEXT, TEXT, TEXT,
  SMALLINT, TEXT[], BOOLEAN)
memory_governance_create_memory(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT,
  TEXT[], TEXT, UUID, UUID, TEXT)
memory_governance_update_memory(UUID, UUID, BIGINT, TEXT, TEXT, TEXT,
  SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT)
memory_governance_delete_memory(UUID, UUID, BIGINT, UUID, UUID, UUID, UUID)
memory_governance_memory_detail(UUID, UUID)
memory_governance_decide_review(UUID, UUID, UUID, TEXT, UUID, TEXT,
  TEXT, TEXT)
memory_governance_list_message_activities(UUID, UUID, INTEGER)
```

Migration `072` replaces only the existing
`memory_governance_decide_review(...)` function body. It adds no table, HTTP
route, function signature, role, or direct table grant.

## 3. Contracts

- User identity comes only from the authenticated Go context. Project,
  Conversation, Memory, Review, source, and assistant IDs are re-bound to that
  user in SQL.
- Project mutation is revision-fenced. Conversation membership and `Use` /
  `Learn` modes are `memory_scope_generation`-fenced. Archiving a Project makes
  effective Learn false without rewriting the user's stored mode.
- Scoped Memory mutation requires an exact scope shape, current owner,
  current Project/Conversation generation, current visibility epoch, active
  lifecycle, and expected revision. A pure scope move appends operation `move`
  while preserving confirmed/AI authority, fact/subject keys, confidence, and
  temporal metadata. A no-op does not advance revision.
- Go and SQL independently classify Memory content. `secret` is rejected;
  content classified `sensitive` requires `sensitive_memory_enabled`, even if
  the client sends `sensitivity=normal`. Review `edit_merge` follows the same
  classification rule.
- The legacy `/v1/memories` API stays Global-only. After `060`, its repository
  must call the two `memory_governance_*_global_legacy` wrappers. Direct runtime
  EXECUTE on `memory_upsert_global_manual` and
  `memory_update_global_manual` is revoked so normal-looking client input cannot
  bypass SQL classification.
- Governance snapshot/detail may hydrate plaintext only from a current,
  enabled, active, epoch- and scope-generation-authorized row. Deleted,
  disabled, archived, expired, superseded, or stale rows return a marker/link,
  not revision-reconstructed plaintext.
- Deleted source Conversations expose only `sourceDeleted=true`. Revision
  snapshots already purged expose `purged=true` without `priorContent`.
- Review decisions are user actions. Every decision rechecks current-user
  ownership, pending status, 30-day expiry, decision shape, and replay hash.
  `keep_current`, `accept_new`, `edit_merge`, and `keep_both` additionally
  recheck epoch, scope generation, and exact target existence/lifecycle/
  revision; write decisions also recheck user evidence and Sensitive authority.
  `reject` is deliberately different: it never consumes or mutates canonical
  Memory, so migration `072` allows it after epoch/scope/target drift. It may
  only wipe the candidate, complete the link-only Activity, and append the
  existing IDs/hash/result-only decision audit.
- Message Activity is assistant-ID scoped, bounded to 20, link-only at rest,
  and current-content hydrated at read time. The frontend polls only while the
  answer is visible and the page is active, stops on terminal state or after
  15 empty polls, and sends `subjectRevision` for undo.
- Delete remains immediate-hide plus provider-free online purge. Backup status
  becomes `retention_expired` after the eight-week deadline; PR9 does not add
  off-host pruning or encrypted Export/Import.
- Local Memory remains hidden, not deleted, in Server mode. No PR9 test calls a
  live Provider or reads/writes live user Memory.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Cross-user Project/Conversation/Memory/Review/assistant ID | Not found or bounded governance validation error; no data hydration/mutation. |
| Project revision or Conversation scope generation changed | `MEMORY_GOVERNANCE_REVISION_STALE` or `MEMORY_GOVERNANCE_SCOPE_STALE`; reload before retry. |
| Project archived | Existing membership remains visible, effective Learn is false, and new assignment is rejected. |
| Secret-like create/update/edit-merge | `MEMORY_GOVERNANCE_MEMORY_INVALID`; no plaintext row/audit copy. |
| Sensitive content while Sensitive is off | `MEMORY_GOVERNANCE_SENSITIVE_DISABLED`, regardless of client label. |
| Same-scope exact active Memory exists | `MEMORY_GOVERNANCE_EXACT_CONFLICT`; cross-scope exact content remains legal. |
| Detail/Activity target is no longer current | Return deleted/unavailable marker and no Memory/revision plaintext. |
| Source Conversation was deleted | Evidence returns `sourceDeleted=true` and no source excerpt/title body. |
| Review is expired, already decided, malformed, cross-user, or has a conflicting replay hash | Reject with Review stale/not-found/replay-conflict behavior and no canonical mutation. |
| Pending unexpired `reject` after epoch, scope, or target revision drift | Return `rejected/USER_REJECTED`; wipe candidate plaintext, append one plaintext-free decision, and leave canonical Memory byte-authoritative. |
| `keep_current`, accept, merge, or keep-both after epoch/scope/target drift | `MEMORY_GOVERNANCE_REVIEW_STALE` or `MEMORY_GOVERNANCE_SCOPE_STALE`; no candidate or canonical mutation. |
| Old v1 write function under `go_api_runtime` after `060` | Permission denied. |
| Down sees Review decisions, decided legacy Reviews, or `move` revisions | Fail closed with a `MEMORY_GOVERNANCE_ROLLBACK_*` code. |

## 5. Good / Base / Bad Cases

- **Good**: move a confirmed Global Memory to an active Project with the
  current revision. Revision advances once with operation `move`; authority,
  fact key, and temporal data survive; the row remains governance-only.
- **Good stale Review**: a target advances from revision one to revision two;
  rejecting the still-pending candidate succeeds, purges only candidate
  plaintext, and leaves the target hash/revision unchanged.
- **Base**: list an empty governance snapshot. Settings and empty arrays are
  returned, the assistant chip performs bounded empty polling, and no Provider
  or prompt behavior changes.
- **Bad**: trust `sensitivity=normal`, call the pre-`060` legacy write function,
  hydrate Activity content from an old revision, use current Memory revision
  instead of Activity `subjectRevision` for undo, bypass target fences for an
  accepting/merging/keep-current decision, or let a scoped row enter the v1
  prompt/Usage reader.

## 6. Tests Required

- Static SQL tests assert every function signature, pinned SECURITY DEFINER
  search path/owner, exact grants/revokes, plaintext-free Review audit, read
  hydration fences, secret/Sensitive classifiers, and rollback guards.
- Disposable PostgreSQL 17 must prove `059 -> 060 -> 059 -> 060`, cross-user
  denial, stale revision/generation rejection, archive Learn override, pure
  move preservation/no-op behavior, Review decision replay/conflict, deleted
  source/history markers, scoped provider-free purge, retention expiry, legacy
  old-function denial/new-wrapper behavior, and table CRUD denial.
- Migration `072` additionally requires `071 -> 072 -> 071 -> 072`: prove a
  stale-target reject fails before/after down, succeeds after up/re-up through
  `go_api_runtime`, leaves target hash/revision unchanged, wipes candidate
  plaintext, writes exactly one replay-safe audit, and keeps every target-
  consuming decision stale-fenced.
- Go tests cover strict JSON/UUID/limit validation, settings extension,
  Global-only legacy compatibility, secret/Sensitive classification, policy
  gating of the v1 reader, Activity mapping, and error status mapping.
- Frontend tests cover server-only composition, typed URLs/bodies, local adapter
  denial, Activity terminal/summary/undo revision behavior, loading/empty/error
  states, accessible names, and responsive source composition.
- Run focused race, all backend tests/vet, frontend format/lint/typecheck/tests/
  build, Compose/preflight tests, backend image build, security scan, and
  `verify-standalone.sh --full`.

## 7. Wrong vs Correct

### Wrong

```go
row := db.QueryRowContext(ctx, `SELECT * FROM memory_update_global_manual(...)`)
```

This preserves a runtime bypass around migration `060` classification and will
also fail with permission denied once the intended grant is active.

### Correct

```go
memory, err := queryGovernanceJSON[GovernanceMemory](ctx, repository, `
SELECT memory_governance_update_global_legacy(...)
`)
```

The wrapper reclassifies content under the pinned SQL authority, enforces the
Sensitive gate, rejects secrets, and then returns the current governance row.
The Go service still performs its own earlier classification for fail-fast and
Provider/transport containment.

For stale Review rejection, do not refresh stored target authority or weaken
all decision fences:

```sql
-- Wrong: this lets an accept/merge use stale target authority.
UPDATE user_memory_review_targets
SET expected_revision = current_memory.revision;

-- Correct: only the non-canonical reject branch skips currentness checks.
IF p_decision_kind <> 'reject' THEN
  -- epoch, scope-generation, target lifecycle/revision fences
END IF;
```

Reject is safe to unblock because it deletes candidate authority rather than
creating, superseding, or selecting canonical Memory. The pending/expiry/user/
replay checks remain outside and before this exception.

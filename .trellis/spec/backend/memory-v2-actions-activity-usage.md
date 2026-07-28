# Memory v2 direct action, Activity, and Usage contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`057_memory_actions_activity_usage`, direct-user Memory intent/planning,
Memory task-model resolution, assistant Memory Usage persistence, Activity
polling, or revision-safe Activity undo.

PR6 keeps the v1 Global reader and existing Memory CRUD payloads. It does not
add the PR9 frontend, Review accept/reject, Project settings CRUD, a v2 reader,
BM25/vector retrieval, L2/L3, Export/Import, or Hindsight.

## 2. Runtime flow

```text
current completed role=user message
  -> deterministic lexical gate fixes remember|correct|forget intent
  -> local secret rejection before planner egress
  -> bounded current Memory hydration
  -> strict versioned Provider JSON proposal
  -> Go rebinds visible targets/revisions and semantic scope kind
  -> PostgreSQL rechecks user, source, assistant, scope generation,
     visibility epoch, lifecycle, target, revision, and exact conflicts
  -> one canonical action or hash/ID-only terminal result

v1 Recall result actually injected into answer context
  -> assistant finalize transaction
  -> at most five immutable L1 Usage links

direct action / PR5 Review outcome / job dead-letter
  -> link-only Activity row
  -> authenticated cursor polling
  -> revision-safe undo for created/corrected direct actions only
```

Planner/action failure does not fail the main chat request. The result is a
bounded degradation code or action-result object in assistant metadata;
durable Activity remains PostgreSQL-authoritative.

## 3. Signatures

Go API-owned capabilities:

```text
memory_hydrate_direct_user_action(UUID, UUID, UUID, UUID)
memory_apply_direct_user_action(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID,
  UUID, UUID, UUID, UUID, SMALLINT,
  TEXT, TEXT, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, TEXT,
  DOUBLE PRECISION, JSONB, TEXT, TEXT
)
memory_record_message_usages(UUID, UUID, UUID, JSONB)
memory_list_activities(UUID, UUID, INTEGER)
memory_list_message_usages(UUID, UUID)
memory_undo_activity(UUID, UUID, BIGINT, UUID, UUID, UUID, UUID)
```

The planner output is exactly:

```json
{
  "schemaVersion": "neo-chat.memory-user-action.v1",
  "action": "remember|correct|forget",
  "memoryType": "fact|preference|instruction|project|warning|decision|context|null",
  "content": "string|null",
  "importance": 1,
  "tags": [],
  "sensitivity": "normal|sensitive|secret",
  "scopeType": "global|project|conversation",
  "confidence": 0.0,
  "targets": [{"memoryId": "uuid", "expectedRevision": 1}]
}
```

Every key is required, including nullable keys. Unknown, missing, duplicate,
and trailing JSON is invalid. Output is at most 16 KiB and targets are at most
five; a mutation requires exactly zero targets for remember or exactly one
current target for correct/forget.

HTTP governance seams are:

```text
GET  /v1/memory-activities?cursor=<uuid>&limit=1..100
POST /v1/memory-activities/{activityId}/undo
     {"expectedRevision": <positive integer>}
GET  /v1/memory-usages?assistantMessageId=<uuid>
```

## 4. Contracts

### Authority

- Only the current source message passed by the chat handler may cross the
  lexical gate. It must be same-user, completed, undeleted, `role=user`, and
  the parent of the current streaming assistant. Assistant text, history,
  stored Memory commands, system prompts, Web, Knowledge, attachments, and
  tool output never set direct-action intent.
- The planner receives the fixed intent, the current user text after local
  privacy filtering, and at most 20 current visible Memory rows. It never
  receives an authoritative user, Project, or Conversation ID.
- Planner calls prefer `task_model_settings.memory`. Only an absent/empty
  Memory task setting falls back to the already-resolved chat Provider/model.
  A configured malformed, disabled, deleted, or unavailable Provider/model
  fails the action without silent fallback.
- Go accepts targets only from the hydrated visible set and replaces model
  revisions with the hydrated current revisions. A forged ID, stale revision,
  or scope mismatch becomes `review_required` and is not persisted as a
  normalized target.
- PostgreSQL derives the authenticated user, current Conversation Project,
  Project/Conversation generation, and visibility epoch. It repeats current
  enabled/lifecycle/generation/epoch/revision checks before mutation.
- `remember` creates one active canonical row with `source=direct_user`,
  `authority_kind=direct_user`, confidence one, current user evidence, and the
  chosen current scope. A same-scope exact current row is `EXACT_NOOP` and
  creates no Activity. A non-current exact row is a conflict, not a new row.
- `correct` never changes scope. It requires one current target, blocks another
  same-scope exact row, appends a complete typed prior snapshot at the new
  revision, then updates canonical content/source/authority.
- `forget` calls the same tombstone/manifest/outbox/provider-free purge
  authority as normal delete. It cannot clear an old tombstone or restore the
  deleted row. Rebuild creates a new canonical ID.
- A locally detected or model-declared secret is rejected before canonical
  mutation. The action stores only IDs, SHA-256, bounded result code, and time;
  it stores no candidate content, normalized text, tags, or credential.
- Confidence below `0.80`, unavailable scope, zero/multiple mutation targets,
  stale target, forged target, or exact conflict is `review_required`. PR6
  stores an action/Activity result only; it does not accept or reject PR5
  Review suggestions.

### Usage and Activity

- `message_memory_usages` stores only assistant ordinal, user, L1 Memory ID,
  injected revision, layer/scope/purpose, and time. It stores no query,
  content, embedding, prompt, rerank score, or raw score.
- Usage is part of assistant finalization. A stale/deleted/disabled/invisible
  target or wrong scope fails the transaction, leaving the assistant streaming
  and writing no partial Usage or capture event.
- One assistant's Usage list is immutable. A per-assistant advisory lock
  serializes retries. An exact same ordered replay succeeds; changed identity,
  revision, scope, order, or length fails with
  `MEMORY_USAGE_REPLAY_CONFLICT`.
- `message_memory_activities` stores subject/source links, revision, action,
  bounded status/reason/undo fields, and time only. Current content is hydrated
  at read time under the requesting user.
- Direct applied/review/rejected/failed actions, PR5 pending/rejected Review
  suggestions, and Memory job dead-letter transitions create Activity.
  `EXACT_NOOP` is intentionally silent.
- Activity polling is ascending `(created_at, id)` cursor order. A cursor is
  valid only for the authenticated user. Each assistant has at most 64
  Activity ordinals and each page has at most 100 rows.
- Activity/Usage content is returned only while the canonical row is current:
  undeleted, enabled, active lifecycle, live visibility epoch, and live Global/
  Project/Conversation generation. Otherwise the API returns a deleted marker
  and never reconstructs content from revisions.

### Undo

- Undo accepts an Activity ID plus the Activity's exact subject revision. Only
  direct-action `created` and `corrected` Activities with `undo_status=available`
  are eligible.
- Created undo succeeds only while the canonical row remains at the created
  revision and every current epoch/scope/lifecycle fence passes. It performs
  normal Forget, including tombstone, manifest, outbox, and purge job.
- Corrected undo succeeds only while the canonical row remains at the corrected
  revision, the complete prior typed snapshot remains unpurged, its source
  Conversation/user message still exists and is active, and restoring it would
  not create an exact-scope conflict.
- Corrected undo appends a `restore` revision containing the corrected state,
  then restores every typed prior field and increments revision. Revision rows
  remain append-only.
- Current revision/epoch/scope drift becomes Activity
  `status=review_required`, `reason_code=UNDO_STALE`; missing/purged source
  snapshot or exact conflict becomes the corresponding bounded Review result.
  Undo never force-overwrites later changes.
- Forgotten direct actions are not undoable. The user must explicitly create a
  new canonical row, preserving the old tombstone.

### Privilege and rollback

- `go_api_runtime` can execute only the six public PR6 capabilities. It has no
  direct action/target/Activity/Usage/revision table write permission.
- `memory_worker_runtime` receives no PR6 action, Activity, Usage, or undo
  execute permission and no direct table access.
- Internal helper and trigger functions are not granted to runtime logins.
  All `SECURITY DEFINER` functions pin the application schema, `pg_catalog`,
  and `pg_temp`; their owner cannot create schema objects.
- Provider-free purge clears every PR6 typed prior snapshot field together
  with the prior content and marks it `ONLINE_PURGED`. The append-only guard
  permits only that complete one-way wipe.
- Down fails when any action, target, Activity, Usage, typed revision snapshot,
  or `source=direct_user` canonical row exists. Never delete user history to
  bypass the guard. Clean `056 -> 057 -> 056 -> 057` must replay exactly.

## 5. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Source is not current completed user parent | No planner mutation; SQL capability rejects invalid source/message authority. |
| Secret assignment/token appears in current message | Zero planner calls; hash-only `SECRET_REJECTED`. |
| Planner JSON is missing/unknown/duplicate/trailing/oversized | Hash-only failed action or bounded degradation; chat continues. |
| Task model is absent | Use current chat Provider/model. |
| Task model is configured but invalid/unavailable | `PLANNER_PROVIDER_FAILED`; never fall back silently. |
| Target ID is not in hydrated context | `TARGET_INVALID`, no spoofed target row. |
| Target revision differs | `REVISION_STALE`, no canonical mutation. |
| Project scope is requested without a current Project | `SCOPE_UNAVAILABLE`, nullable resolved IDs, no canonical mutation. |
| Same-scope exact current remember | `EXACT_NOOP`, action row only, no Activity. |
| Usage retry differs from committed list | `MEMORY_USAGE_REPLAY_CONFLICT`, committed links unchanged. |
| Memory is deleted or epoch/generation hidden | Activity/Usage return deleted marker and no content. |
| Undo target changed after Activity | `review_required`; later canonical revision remains unchanged. |
| Correction snapshot was purged/source deleted | `UNDO_SNAPSHOT_UNAVAILABLE`; no restoration. |
| Down sees any PR6 authority/history | `MEMORY_ACTION_ROLLBACK_REQUIRES_*`; schema retained. |

## 6. Good/Base/Bad Cases

- **Good**: a current completed user message says to remember a new scoped
  fact; the strict planner returns one valid proposal, Go binds the current
  scope, SQL applies it, assistant finalization records the exact injected
  revisions, and the resulting Activity can be undone while its revision is
  still current.
- **Base**: the same-scope normalized Memory already exists; the action returns
  `EXACT_NOOP`, creates no Activity, leaves canonical/revision state unchanged,
  and the main chat response still completes.
- **Bad**: assistant/tool text supplies an instruction, the planner forges a
  target or scope, Usage replay changes order, or undo sees a newer revision;
  the boundary fails closed with a bounded hash/ID-only result and never
  overwrites canonical state or committed Usage.

## 7. Tests Required

- Go: lexical positive/negative cases, completed-current-user-only authority,
  strict recursive JSON, target/scope/revision spoof rebinding, secret zero
  planner egress/plaintext, task-model preference/fallback, metadata bounds,
  HTTP polling/Usage/undo validation, and Memory worker strict/privacy
  regression.
- PostgreSQL 17: full `001 -> 057`, Project-scope unavailable Review,
  remember/correct/forget, exact NOOP silence, complete snapshot safe undo,
  stale undo Review, Usage finalize rollback and immutable replay, current-state
  deleted marker, PR5 pending/rejected and dead-letter Activity, provider-free
  direct purge, role denial, guarded down, clean down, and re-up.
- Static migration: tables contain no query/prompt/content copy, every generic
  link has ownership authority, function signatures/grants are exact, revision
  wipe is complete, triggers/backfill are bounded, and both down guards exist.
- Run focused race, all backend tests, `go vet ./...`, migration/preflight/
  Compose, backend image, frontend/RAG regression, and full standalone gates.
  No PR6 test calls a Live Provider or touches Live user Memory.

## 8. Wrong vs Correct

### Wrong

```text
assistant or tool text says "forget it"
  -> model emits target UUID/project UUID
  -> API trusts it and updates canonical
  -> retry rewrites Usage rows
  -> stale UI undo overwrites a later correction
```

### Correct

```text
current completed user text passes lexical gate
  -> strict proposal over bounded redacted context
  -> Go rebinds visible targets
  -> SQL repeats current authority fences
  -> canonical mutation or hash/ID-only Review result
  -> finalize atomically records immutable injected revisions
  -> link-only Activity polls and stale undo fails closed
```

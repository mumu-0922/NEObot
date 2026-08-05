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
  -> exact referential remember may select the nearest preceding completed
     role=user message as candidate facts only
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

Before the normal answer request, a direct-action result also becomes one
bounded server-authored System instruction. It contains only a closed mapping
of action/status to user-facing outcome: `applied` and `noop` must be confirmed;
`rejected`, `review_required`, `failed`, and local degradation must not claim
success. It contains no Memory content, IDs, revision, hash, or raw result code,
and it forbids the answer model from claiming that it lacks a Memory Tool or
permission. Ordinary turns receive no instruction and remain unchanged.

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

The Provider planner must call the versioned required Tool
`propose_memory_action_v1` exactly once. The Tool arguments contain the
semantic fields below except `schemaVersion`; the server binds
`schemaVersion=neo-chat.memory-user-action.v1` from the versioned Tool name and
then materializes this exact canonical proposal:

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

The Planner Tool round disables optional thinking, uses `temperature=0`, and
caps output at 1,024 tokens plus the existing 16-KiB argument boundary. Plain
assistant text, zero/multiple calls, a different Tool name, malformed or extra
arguments, a model-supplied `schemaVersion`, and semantic validation failure
are `PLANNER_OUTPUT_INVALID`. Provider construction, Tool-round startup,
transport, timeout, or stream failure is `PLANNER_PROVIDER_FAILED`. Neither
failure mutates canonical Memory or fails the ordinary answer.

Ordinary direct actions retain the planner input identity
`neo-chat.memory-user-action-input.v1`. An exact bounded referential remember
command uses `neo-chat.memory-user-action-input.v2` and adds exactly one
factual-reference field:

```json
{
  "schemaVersion": "neo-chat.memory-user-action-input.v2",
  "detectedIntent": "remember",
  "currentUserMessage": "那你写进去呀",
  "referencedPreviousUserMessage": "我喜欢喝生椰拿铁",
  "projectScopeAvailable": false,
  "currentMemories": []
}
```

The v2 input does not change the exact v1 planner output or PostgreSQL
function signatures. Its request hash binds a fixed reference-hash version,
the current command, and the referenced user text; v1 request hashing remains
byte-compatible.

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

- Only the current source message passed by the chat handler may set action
  authority or cross the lexical intent gate. It must be same-user, completed,
  undeleted, `role=user`, and the parent of the current streaming assistant.
  Assistant text, history, stored Memory commands, system prompts, Web,
  Knowledge, attachments, and tool output never set direct-action intent.
- An exact bounded referential `remember` command may select only the nearest
  preceding completed, undeleted, same-user `role=user` row before the current
  message from the already authorized Conversation list. It skips assistant
  and incomplete rows. That prior row supplies candidate facts only; the
  current message remains the SQL source and assistant parent. A missing prior
  user row performs no planner call and no mutation. A full-fact current
  command stays on v1 and never mixes in history.
- The referential set includes standalone `记住`, `记住它/这个/这条`,
  `记下来`, `记一下`, the longer explicit previous-message forms, and the
  bounded English equivalents. Generic standalone `保存` and `写进去` remain
  non-actions. An anchored referential match runs before the broad remember
  gate so a bare `记住` cannot enter schema v1 without a factual source.
- The planner receives the fixed intent, the current user text after local
  privacy filtering, the optional separately named prior-user factual
  reference, and at most 20 current visible Memory rows. It never receives an
  authoritative user, Project, Conversation, or referenced-message ID.
  Referential remember facts may come only from the prior-user field, never
  from assistant text, current Memories, or the referential command itself.
- Planner calls prefer `task_model_settings.memory`. Only an absent/empty
  Memory task setting falls back to the already-resolved chat Provider/model.
  A configured malformed, disabled, deleted, or unavailable Provider/model
  fails the action without silent fallback.
- The planner uses one named `required` Tool rather than free-form JSON. The
  Tool name owns the output schema version, its strict argument schema pins the
  already-detected action, and Go still performs exact-key and semantic
  validation after the Provider boundary. A Provider that lacks the native
  Tool-round contract fails closed without a plain-chat fallback.
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
- A locally detected secret in either the current action or referenced prior
  user text is rejected before planner egress. Both fields reuse deterministic
  Provider redaction; a fully redacted reference is `REFERENCE_REDACTED` with
  zero Provider calls. A locally detected or model-declared secret is rejected
  before canonical mutation. The action stores only IDs, SHA-256, bounded
  result code, and time; it stores no candidate content, normalized text, tags,
  or credential.
- Confidence below `0.80`, unavailable scope, zero/multiple mutation targets,
  stale target, forged target, or exact conflict is `review_required`. PR6
  stores an action/Activity result only; it does not accept or reject PR5
  Review suggestions.
- The answer model does not decide whether a direct action succeeded. A closed
  action/status mapping appended by the server is the only answer authority:
  applied remember/correct/forget and exact NOOP are acknowledged, while
  rejected/review/failed outcomes are described without success, sensitive
  content, internal identifiers, codes, or another Tool request.

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
| Referential remember has no preceding completed user row | No planner call and no mutation. |
| Current command is standalone `记住` after a completed user fact | Use that prior user fact through schema v2; never plan schema v1 from the command alone. |
| Current command is standalone `保存` or `写进去` | No direct action; do not infer a reference from generic writing language. |
| The nearest preceding row is assistant/incomplete/wrong user or Conversation | Skip it; never expose it or use it as candidate facts. |
| Secret assignment/token appears in current message | Zero planner calls; hash-only `SECRET_REJECTED`. |
| Secret appears in referenced prior user text | Zero planner calls; reference-bound hash-only `SECRET_REJECTED`. |
| Referenced prior user text is fully removed by privacy redaction | Zero planner calls; hash-only `REFERENCE_REDACTED`. |
| Current message contains a complete remember fact | Use unchanged v1 input/hash and do not include a prior-message field. |
| Applied/noop direct action reaches the normal answer model | Append a status-only authoritative System instruction; the answer confirms the server result and does not claim missing permission. |
| Rejected/review/failed/degraded direct action reaches the answer model | Append a status-only failure instruction; never claim success or include content/IDs/codes. |
| Ordinary non-action turn | Append no direct-action answer instruction; preserve the existing System prompt. |
| Planner Tool is absent/duplicated/wrong, emits text, or has missing/unknown/duplicate/trailing/oversized arguments | Hash-only `PLANNER_OUTPUT_INVALID`; chat continues. |
| Planner Provider/transport/timeout fails | Hash-only `PLANNER_PROVIDER_FAILED`; chat continues. |
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
- **Good referential**: a current message says “那你写进去呀” or standalone
  “记住”; Go selects the nearest preceding completed user fact, omits
  intervening assistant text,
  redacts the fact into the v2 planner field, and keeps the current message as
  action/SQL source while applying the returned remember proposal. The normal
  answer model receives only a server-authored success instruction and briefly
  confirms the write instead of claiming it lacks permission.
- **Base**: the same-scope normalized Memory already exists; the action returns
  `EXACT_NOOP`, creates no Activity, leaves canonical/revision state unchanged,
  and the main chat response still completes.
- **Bad**: generic “写进去” text, assistant/tool text, or an incomplete prior
  user row supplies authority/facts; the planner forges a target or scope;
  Usage replay changes order; or undo sees a newer revision. The boundary fails
  closed with a bounded hash/ID-only result and never overwrites canonical
  state or committed Usage.

## 7. Tests Required

- Go: lexical positive/negative cases including the exact standalone `记住`
  family and generic `保存`/`写进去` negatives, completed-current-user-only
  authority, referential remember detection, nearest completed user selection, assistant/
  incomplete/cross-authority exclusion, missing-reference silence, full-fact
  v1 isolation, reference-bound hashing, strict recursive JSON, target/scope/
  revision spoof rebinding, current/reference secret zero planner egress,
  partial and full reference redaction, task-model preference/fallback,
  versioned required-Tool framing across GPT/DeepSeek-shaped arguments, server-
  bound schema version, zero/multiple/wrong/plain-text call denial, Provider
  failure classification, metadata bounds, status-only answer instructions for applied/noop/rejected/
  review/failed/degraded outcomes, ordinary-prompt byte preservation, no
  content/ID/code answer egress, HTTP polling/Usage/undo validation, and Memory
  worker strict/privacy regression.
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
  -> exact referential remember optionally selects nearest prior user facts
  -> strict proposal over separately named bounded redacted context
  -> Go rebinds visible targets
  -> SQL repeats current authority fences
  -> canonical mutation or hash/ID-only Review result
  -> finalize atomically records immutable injected revisions
  -> link-only Activity polls and stale undo fails closed
```

Wrong referential handling:

```text
“那你写进去呀” -> regex only -> planner sees no prior fact
all Conversation history -> planner -> assistant text may become Memory
server writes Memory -> answer model is uninformed -> “I lack permission”
```

Correct referential handling:

```text
exact referential command + current Conversation order
  -> nearest preceding completed same-user user row only
  -> Secret classification/redaction before Provider egress
  -> v2 planner input: current command authority + prior user factual reference
  -> current command remains SQL source/assistant parent
  -> status-only server System instruction makes the answer report the result
```

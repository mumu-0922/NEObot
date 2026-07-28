# Server Memory Governance API

## Purpose and release boundary

Migration `060_memory_governance_ui` exposes the authenticated governance
surface for Memory v2 PR9. It lets one user manage Projects, Conversation
Memory policy, scoped canonical Memory, pending Review suggestions,
provenance/history/Usage, deletion progress, and per-answer Activity.

This release does **not** promote the lexical or hybrid reader. The existing v1
Global Top 5 is still the only Memory content injected into prompts and written
to answer Usage. Project and Conversation Memory can be created and governed,
but cannot reach an answer until a later reader-promotion release. An effective
Conversation `Use Memory = off` may only remove the v1 Memory block.

## Authentication and authority

All routes are current-user routes. No request body accepts `userId`.
Project, Conversation, Memory, Review, assistant message, source message,
revision, visibility epoch, and scope generation are rebound and checked by
PostgreSQL SECURITY DEFINER capabilities before content is returned or state is
changed.

`go_api_runtime` receives EXECUTE on the narrow governance capabilities and no
direct CRUD on Projects, Review suggestions, evidence, revisions, diagnostic
tables, or Review decision audit. Migration `060` also revokes runtime EXECUTE
on the old Global manual upsert/update functions and replaces them with
classification-aware legacy wrappers. This keeps `/v1/memories` compatible
without leaving a Sensitive/secret bypass.

## HTTP surface

### Snapshot

```http
GET /v1/memory-governance
```

Returns one bounded object containing current settings, Projects,
Conversations, active governance-visible Memory, pending Reviews, deletion
progress, and sanitized search diagnostics. Diagnostics contain profile,
status, fallback, counts, token estimate, duration, and time only—never query,
Memory content copies, embedding, raw score, prompt, or Provider secret.
Migration `062` extends this response with optional `l2Scene.profile` and
`l2Scene.scenes` governance state.

### L2 Scene governance

```http
GET  /v1/memory-governance/scenes/{sceneId}/details
POST /v1/memory-governance/scenes/{sceneId}/enabled
     {"expectedRevision":1,"enabled":false}
POST /v1/memory-governance/scenes/{sceneId}/rebuild
POST /v1/memory-governance/scenes/rebuild
```

Scenes are rebuildable Global/Project summaries over current same-scope L1
Memory. The snapshot exposes profile/generation/L1-reader readiness plus Scene
scope, topic, content, lifecycle, source watermark, member count, and current-
source status. Detail hydrates a member Memory and its evidence only while the
exact member revision/hash/scope/epoch remains current; otherwise it returns a
changed/deleted marker without reconstructing old plaintext.

There is intentionally no Scene plaintext create or patch route. User
correction goes through the existing scoped L1 Memory create/update UI, which
invalidates and rebuilds the derived Scene. Disable/enable and rebuild are
revision/current-user fenced. Scene promotion remains migration-owner-only and
cannot be invoked through HTTP.

### Projects

```http
GET  /v1/projects
POST /v1/projects
     {"name":"Neo Chat","description":"optional"}

PATCH /v1/projects/{projectId}
      {
        "name":"Neo Chat",
        "description":"optional",
        "expectedRevision":1,
        "lifecycleStatus":"active|archived"
      }
```

PR9 supports archive/restore, not permanent Project deletion. An archived
Project forces effective Learn off for its existing Conversations without
rewriting their explicit `learnMode`. New assignment to an archived Project is
rejected.

### Conversation policy

```http
GET   /v1/chat/conversations/{conversationId}/memory-policy
PATCH /v1/chat/conversations/{conversationId}/memory-policy
      {
        "expectedScopeGeneration":1,
        "projectId":"uuid-or-empty",
        "useMode":"inherit|on|off",
        "learnMode":"inherit|on|off"
      }
```

Changing Project membership or policy advances the Conversation Memory scope
generation. A stale generation is rejected; the client reloads rather than
overwriting a newer policy.

### Scoped Memory

```http
POST /v1/memory-governance/memories
PATCH /v1/memory-governance/memories/{memoryId}
{
  "type":"fact|preference|instruction|project|warning|decision|context",
  "content":"1..2000 characters",
  "importance":1,
  "tags":[],
  "expectedRevision":1,
  "scopeType":"global|project|conversation",
  "projectId":"uuid-or-empty",
  "conversationId":"uuid-or-empty",
  "sensitivity":"normal|sensitive"
}

DELETE /v1/memory-governance/memories/{memoryId}
{"expectedRevision":1}

GET /v1/memory-governance/memories/{memoryId}/details
```

Create omits `expectedRevision`; update and delete require it. Scope shape is
exclusive. A pure move preserves canonical authority and semantic/temporal
metadata and appends a `move` revision. An unchanged update is a no-op and does
not advance revision.

The server reclassifies content independently of the client label. Secret-like
content is rejected before persistence. Sensitive content requires the user's
Sensitive switch even when the client sends `normal`.

Detail returns the current Memory plus bounded evidence, revision history, and
Usage links. Evidence from a deleted source returns only `sourceDeleted=true`.
Purged history returns only the revision metadata and `purged=true`. The API
never reconstructs deleted or unauthorized current content from a prior
revision snapshot.

### Review decisions

```http
GET  /v1/memory-reviews
POST /v1/memory-reviews/{suggestionId}/decision
     {
       "decision":"keep_current|accept_new|edit_merge|keep_both|reject",
       "editedContent":"required only for edit_merge"
     }
```

The decision rechecks pending status, 30-day expiry, user/epoch/scope/target
authority, evidence, Sensitive policy, and replay hash. Successful decisions
clear candidate plaintext and retain only ID/hash/result audit. `edit_merge`
is classified again; a normal client label cannot conceal Sensitive or secret
content.

### Answer Activity and undo

```http
GET  /v1/memory-activities?assistantMessageId={uuid}&limit=1..20
POST /v1/memory-activities/{activityId}/undo
     {"expectedRevision":1}
```

Activity is link-only at rest. The read function hydrates content only while
the subject Memory is current, enabled, active, epoch-valid, and scope-
generation-valid. Otherwise it returns a deleted/unavailable marker. The
frontend uses Activity `subjectRevision`, not the currently hydrated Memory
revision, for revision-safe undo.

The assistant chip polls every two seconds only while its answer is near the
viewport and the document is visible. It stops on terminal Activity or after
15 empty/error polls. Activity and candidate/source bodies are not copied into
message metadata or browser persistence.

### Encrypted portability

Migration `061` extends the authenticated Server Memory surface:

```http
POST /v1/memory-export
     {"passphrase":"request-only","includeHistory":true}

POST /v1/memory-import/dry-run
POST /v1/memory-import/confirm
Content-Type: multipart/form-data
package=<encrypted .mm-memory>
passphrase=<request-only>
mappings=<strict JSON>
planToken=<confirm only>
```

Export returns an `application/octet-stream` attachment with `Cache-Control:
no-store`. Import accepts one bounded occurrence of each known form field.
Dry-run returns `NOOP|ADD|REVIEW|REJECT|SCOPE_REQUIRED`, required scope
mappings, settings suggestions, a deterministic plan hash, and a ten-minute
token. Confirm requires the same encrypted package and mappings, reauthenticates
and rebuilds the plan, and writes only unchanged `ADD` rows. Settings are never
applied and conflicts are never overwritten. Duplicate/unknown JSON or form
fields, wrong passphrases, modified ciphertext, cross-user mappings, and state
drift fail closed. Plan/token/state conflicts return `409` and require a new
dry-run.

Passphrases and plaintext archives are request-local only. The browser keeps
the selected `File`, passphrase, and token in transient component state; none
enters Zustand, IndexedDB, or `localStorage`.

## Errors and stale state

Governance validation errors carry stable `MEMORY_GOVERNANCE_*` codes.
Revision, scope generation, Review state, or decision replay drift is a reload
and retry condition. Cross-user IDs behave as not found or bounded validation
failure. Exact conflicts are same-scope only; the same normalized content may
exist in Global, Project, and Conversation scopes.

Deletion immediately hides Memory, creates the existing tombstone/manifest/
purge chain, and exposes online purge plus eight-week backup-expiry status.
PR9 did not prune physical backups or add encrypted Export/Import. Migration
`061` now provides encrypted portability plus operator deletion replay; backup
retention remains an operator CLI rather than an authenticated browser route.

## Rollout and rollback

Apply migration `060` and deploy the matching backend together: the new backend
calls the governance legacy wrappers, while `060` revokes the old runtime write
functions. Do not mix a pre-`060` writer with the post-`060` grant set.

Pre-traffic rollback is allowed only when there are no Review decisions,
decided legacy Reviews, or `move` revisions. Down restores the old v1 function
grants and drops the governance wrappers. After governance history exists,
retain `060` and use a forward fix; never delete user history to force down.

## Verification

- Static migration signature/grant/search-path/rollback assertions.
- Disposable PostgreSQL 17 `059 -> 060 -> 059 -> 060`, runtime role denial,
  Project/policy/scope/Review/Activity/detail/purge cases, and legacy wrapper
  normal/Sensitive/secret cases.
- Focused race plus all backend tests/vet.
- Frontend format/lint/typecheck/Vitest/build with server/local authority and
  Activity tests.
- Compose validation, preflight regression, backend image binary check,
  security scan, and full isolated standalone verification.

PR9–PR11 verification uses synthetic fixtures and disposable PostgreSQL only.
It makes no live Provider call and does not read or mutate live user Memory.

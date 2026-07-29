# usermemory

`usermemory` is the Go authority for optional, durable, per-user Memory in
server mode. It keeps Memory separate from conversation history and derived
conversation summaries.

## Responsibilities

- expose authenticated settings and legacy Global CRUD at
  `/v1/memory-settings` and `/v1/memories`, plus PR9 Project/policy/scoped
  governance, Review/detail, Activity polling, Usage inspection, and safe undo;
- expose authenticated encrypted portability at `/v1/memory-export`,
  `/v1/memory-import/dry-run`, and `/v1/memory-import/confirm` without staging a
  plaintext archive or applying imported settings;
- persist settings and canonical entries from migration `035`, with the
  additive Project/scope foundation from `053` and provenance/delete authority
  from `055`;
- keep the current v1 prompt/Usage reader explicitly Global-only while PR9
  Project/Conversation Memory remains governance-only;
- enforce migration `060` Project revision, Conversation scope-generation,
  scoped Memory revision/epoch/generation, Review replay, current-only
  plaintext hydration, and SQL-side secret/Sensitive authority;
- hydrate and apply explicit current-user `remember|correct|forget` actions
  through migration `057` capabilities without trusting model-supplied user,
  scope, target, or revision authority;
- return current visible Memory details for Activity/Usage links, or a deleted
  marker after deletion, disablement, archive, or generation/epoch drift;
- normalize and validate types, content, importance, tags, and source IDs;
- rank only relevant lexical/CJK matches and return at most five;
- optionally compare that unchanged v1 Top 5 with migration `058` exact/CJK
  BM25 lanes while keeping the shadow out of prompts and Usage;
- optionally run migration `059` exact/BM25/BGE-M3 lanes, deterministic
  RRF(60), BGE rerank, and a 600-target/900-hard token budget as a second
  default-off, zero-injection comparison;
- optionally run migration `062` same-scope L2 Scene exact/BM25/vector
  RRF/rerank retrieval under a separate default-off shadow flag; active mode
  additionally requires the reader flag and database promotion authority;
- expose Scene profile/list/detail, enable/disable, and rebuild governance
  without any direct derived-plaintext mutation route;
- optionally run migration `063` Global stable-L1 L3 Persona exact/BM25/vector
  RRF/rerank retrieval under independent default-off shadow/reader flags;
- expose Persona profile/detail, enable/disable, and rebuild governance without
  any direct derived-plaintext mutation route;
- prevent disabled Memory or disabled auto-record from reading/writing entries.

Provider prompt injection remains in `internal/chat`. Durable extraction runs
in `internal/memoryworker`; PR5 reuses only
`NormalizeCandidateForStorage` and persists candidate-wide shadow/Review
proposals through migration `056`. It no longer calls the legacy
`Service.StoreExtracted` canonical write path.

## Usage

```go
repo := usermemory.NewPostgresRepository(db)
service := usermemory.NewService(repo)
handler := usermemory.NewHandler(service)

matches, err := service.SearchRelevant(ctx, "keep this concise", 5)

// Called only when MEMORY_LEXICAL_SHADOW_ENABLED=true. The first return value
// is still the exact v1 result used by the prompt and Usage recorder.
matches, shadow, err := service.SearchRelevantWithShadow(
    ctx, rawUserText, conversationID, assistantMessageID, 5,
)

// Called only when MEMORY_HYBRID_SHADOW_ENABLED=true. Hybrid final results
// remain diagnostics; matches is still the exact v1 prompt/Usage authority.
matches, hybrid, err := service.SearchRelevantWithHybridShadow(
    ctx, rawUserText, conversationID, assistantMessageID, 5,
)

// Called only when MEMORY_L2_SCENE_SHADOW_ENABLED=true. Shadow returns no
// Scenes. Active results require MEMORY_L2_SCENE_READER_ENABLED plus current
// database authority and are bounded to two Scenes / 500 estimated tokens.
sceneResult, err := service.SearchRelevantL2Scenes(
    ctx, rawUserText, conversationID, assistantMessageID, activeRequested,
)

// Called only when MEMORY_L3_PERSONA_SHADOW_ENABLED=true. Shadow returns no
// Persona. Active results require MEMORY_L3_PERSONA_READER_ENABLED plus current
// database authority and are bounded to one Persona / 300 estimated tokens.
personaResult, err := service.SearchRelevantL3Persona(
    ctx, rawUserText, conversationID, assistantMessageID, activeRequested,
)

// Export writes an age-encrypted stream. Import dry-run returns only a
// deterministic plan; confirm must resubmit the same encrypted package and
// short-lived plan token.
exported, err := service.ExportMemoryPackage(
    ctx, encryptedWriter, passphrase, includeHistory,
)
plan, err := service.DryRunMemoryImport(
    ctx, encryptedReadSeeker, passphrase, mappings,
)
result, err := service.ConfirmMemoryImport(
    ctx, encryptedReadSeeker, passphrase, mappings, plan.PlanToken,
)
```

Identity is always read from `auth.UserFromContext` through
`auth.UserOrDevelopment`; API inputs never accept a user ID.

Migration `053` allows the same normalized content in different scopes. The v1
repository remains Global-only. Migration `055` moves manual upsert/update and
delete behind narrow database capabilities: edits append one prior snapshot,
and delete atomically hides the row, writes a targeted tombstone and ID/hash-only
manifest, and enqueues a provider-free purge job. HTTP request and response
shapes remain unchanged.

Migration `057` adds an optional `ActionRepository`. Chat can use it to hydrate
a bounded current Memory context and submit a typed direct-user action. The
service normalizes candidate data, rejects secrets locally, binds generated
IDs, and leaves ownership/scope/revision revalidation to narrow PostgreSQL
capabilities. `GET /v1/memory-activities`,
`POST /v1/memory-activities/{id}/undo`, and
`GET /v1/memory-usages?assistantMessageId=...` are backend-only governance
seams until the PR9 frontend.

Migration `058` adds a rebuildable `user_memory_search_projections` table and
ID/revision/rank-only lexical shadow observations. Projection maintenance is
transactional and independent of the rollout flag. The optional
`LexicalShadowRepository` calls one `go_api_runtime` capability after the v1
reader completes; any compare failure becomes a bounded summary and cannot
change v1 items, prompt text, Usage, or chat success.

Migration `059` adds a fixed `siliconflow_bge_m3_v1` 1024-dimensional vector
projection, partial HNSW cosine index, and leased embedding jobs. The Memory
Worker claims those jobs only when the shared hybrid flag is true and can read
one current Memory plus one exact attested `RAG:SILICONFLOW` credential only
through narrow capabilities. The raw canonical hash remains the lease
authority, while the embedding body is a transient deterministic
secret-redacted copy; a fully removed body is terminally failed without a
Provider call. Hybrid prepare keeps the raw query only for SQL source/hash and
lexical authority, while query embedding and authorized RRF rerank use
secret-redacted transient copies. Durable rows keep only hashes, IDs, revisions,
lane ordinals, counts, bounded status, token estimates, and duration.

Migration `060` adds the authenticated governance snapshot; Project
create/list/edit/archive/restore; Conversation Project membership and tri-state
Use/Learn policy; scoped Memory create/update/move/forget/detail; pending Review
decisions; and assistant-message Activity hydration for the frontend. All
plaintext reads require current enabled/lifecycle/epoch/scope-generation
authority. Deleted sources and purged revisions return markers only. The v1
Global repository now calls classification-aware governance wrappers because
`go_api_runtime` no longer has EXECUTE on the old manual upsert/update
functions. This closes the SQL Sensitive/secret bypass without promoting the
v2 reader.

Migration `061` adds canonical JSONL portability inside a pinned
`filippo.io/age v1.3.1` scrypt-authenticated stream. Export reads one
`REPEATABLE READ, READ ONLY` snapshot twice so manifest counts/hash and emitted
records cannot drift. Import fully authenticates and validates bounded records,
rejects secrets before persistence, maps every external scope back to the
current user, and classifies each Memory as
`NOOP|ADD|REVIEW|REJECT|SCOPE_REQUIRED`. Confirm re-authenticates the same
encrypted package and binds user/package/manifest/mappings/plan/current-state
hashes through a ten-minute HMAC token. It atomically writes only `ADD` rows
with fresh local IDs and explicit import authority; settings remain a visible
suggestion only.

The migration also supports an encrypted, plaintext-free deletion package for
offline restore. `ReplayEncryptedDeletionPackage` authenticates the complete
stream before one transaction applies ID+content-hash-fenced hides/wipes and
then rebuilds projections. The operator CLI requires stdin passphrases and an
explicit `--backend-stopped` assertion. Runtime roles have no direct CRUD on
the import/replay tables, and no portability path calls a Provider or changes
the v1 Global Top 5 prompt/Usage reader.

Migration `062` adds rebuildable Global/Project L2 Scenes, exact/CJK BM25 and
fixed BGE-M3 projections, content-free search observations, promotion events,
and Scene governance. Conversation L1 never becomes a Scene member. Every
candidate and final result is authorized against current member revisions,
hashes, scope generation, visibility epoch, Sensitive policy, and the active
L2 generation. Shadow mode stores only bounded diagnostics and returns no
prompt content. Active mode appends a separate untrusted
`<relevant-user-scenes>` block; L1 remains the only Usage authority and every
L2 failure falls back to the unchanged L1 prompt.

Migration `063` adds one rebuildable L3 Persona per user generation from
current stable Global L1 only, with exact/CJK BM25 and fixed BGE-M3 projection,
content-free search observations, promotion events, and Persona governance.
Every member pins its L1 revision/hash, visibility epoch, generation, and
source watermark. Shadow mode returns no prompt content. Active mode requires
the independent reader flag, database promotion, current L1 hybrid authority,
current members, and Sensitive policy before appending a separate untrusted
`<relevant-user-persona>` block. L1 remains the only Usage authority and every
L3 failure falls back to unchanged L1/L2 behavior.

## Main API

| Boundary                            | Purpose                                                     |
| ----------------------------------- | ----------------------------------------------------------- |
| `NewPostgresRepository(*sql.DB)`    | Postgres persistence scoped to the current user             |
| `NewService(Repository)`                    | Validation, settings, CRUD, relevance, and optional action authority |
| `NewHandler(*Service)`                      | JSON HTTP routes and bounded error mapping                          |
| `SearchRelevant(ctx, query, limit)`         | Relevant-only Top-5 retrieval                                      |
| `SearchRelevantWithShadow(ctx, query, conversationID, assistantMessageID, limit)` | Unchanged v1 Top 5 plus sanitized comparison diagnostics |
| `SearchRelevantWithHybridShadow(ctx, query, conversationID, assistantMessageID, limit)` | Unchanged v1 Top 5 plus default-off hybrid diagnostics |
| `SearchRelevantL2Scenes(ctx, query, conversationID, assistantMessageID, activeRequested)` | Default-off Scene shadow or current authorized active results |
| `SearchRelevantL3Persona(ctx, query, conversationID, assistantMessageID, activeRequested)` | Default-off Persona shadow or current authorized active result |
| `HydrateDirectAction(ctx, input)`           | Bounded current context for the strict direct-user planner          |
| `ExecuteDirectAction(ctx, input)`           | Local validation plus narrow SQL action capability                  |
| `ListActivities` / `ListMessageUsages`      | User-scoped polling and answer provenance                           |
| `UndoActivity(ctx, id, expectedRevision)`   | Revision-safe created/corrected undo                                |
| `GovernanceSnapshot(ctx)`                   | Current-user settings/Project/policy/Memory/Review/operation view   |
| `CreateProject` / `UpdateProject`            | Revision-fenced Project create/archive/restore                      |
| `GetConversationPolicy` / `UpdateConversationPolicy` | Generation-fenced Project membership and Use/Learn modes   |
| `CreateGovernanceMemory` / `UpdateGovernanceMemory` / `DeleteGovernanceMemory` | Scoped revision-fenced governance writes |
| `GovernanceMemoryDetail` / `DecideMemoryReview` / `ListMessageActivities` | Current-only detail, Review, and chip hydration |
| `ExportMemoryPackage`                       | Current-user canonical JSONL inside an authenticated age stream      |
| `DryRunMemoryImport`                        | Strict authenticated parse, scope mapping, and deterministic plan    |
| `ConfirmMemoryImport`                       | HMAC/state-fenced atomic ADD-only import                              |
| `ExportDeletionPackage` / `ReplayEncryptedDeletionPackage` | Off-host deletion authority and provider-free restore replay |
| `NormalizeCandidateForStorage(in)`          | Shared validation/normalization without a write                     |

## Files

```text
handler.go               HTTP contract and DTO mapping
repository_postgres.go   user-scoped canonical writes and deletion capability
governance.go             PR9 validation, DTOs, Review hashing, and service operations
governance_handler.go     PR9 Project/scoped Memory/Review HTTP routes
governance_repository_postgres.go  migration-060 governance capabilities
portability_crypto.go     pinned age scrypt stream boundary
portability_package.go    canonical bounded JSONL parser/writer
portability_export.go     repeatable-read export orchestration
portability_plan.go       mapping normalization and short-lived HMAC plan tokens
portability_service.go    deterministic dry-run and ADD-only confirm
portability_handler.go    strict JSON/multipart HTTP portability routes
portability_repository_postgres.go  migration-061 snapshot/apply/replay capabilities
deletion_portability.go   deletion JSONL export/authenticate/replay orchestration
action_repository_postgres.go  PR6 action, Activity, Usage, and undo capabilities
lexical_shadow.go        fail-open PR7 orchestration and diagnostic sanitization
lexical_shadow_repository_postgres.go  narrow migration-058 compare capability
hybrid_shadow.go         fail-open PR8 embedding/RRF/rerank/budget orchestration
hybrid_shadow_repository_postgres.go  narrow migration-059 prepare/record capabilities
l2_scene.go              fail-open PR11 Scene embedding/RRF/rerank/budget orchestration
l2_scene_repository_postgres.go  narrow migration-062 Scene search capabilities
l3_persona.go            fail-open PR12 Persona embedding/RRF/rerank/budget orchestration
l3_persona_repository_postgres.go narrow migration-063 Persona search capabilities
../memoryworker/embedding_worker.go  lease/source/redaction-fenced embedding orchestration
action_service.go         direct action validation and application IDs
privacy.go                shared secret/Sensitive classification and redaction
service.go               validation, settings, normalization, relevance
types.go                 repository/service contracts
uuid.go                  application-owned UUID generation
*_test.go                 unit, HTTP, and optional live Postgres coverage
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/memory-governance-api.md`](../../../docs/contracts/memory-governance-api.md).

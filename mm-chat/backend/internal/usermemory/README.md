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
  RRF(60), migration `064` local pre-rerank admission, request-local BGE score
  abstention, and a 600-target/900-hard token budget as a second default-off,
  zero-injection comparison;
- under the explicit schema-v4 Development profile only, run the strict cloud
  candidate judge concurrently with BGE rerank and intersect ordinal selection
  with BGE order before the existing token selector;
- preserve schema-v6 as failed Development preflight evidence and support the
  schema-v7 first-ToolRound calibration policy through the compatibility route
  interface;
- after a valid product first-round `search_memory({})` call, run the fixed
  hybrid reader without v1 fallback, record the final set, hydrate it through
  migration `065`, recheck identity/redaction, and return at most five bounded
  current-authorized Memory bodies to `internal/chat`;
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

// Called only when MEMORY_HYBRID_SHADOW_ENABLED=true. A valid versioned
// relevance policy is also required; otherwise no hybrid Provider call is
// made. Hybrid final results remain diagnostics and matches is still exact v1
// authority.
matches, hybrid, err := service.SearchRelevantWithHybridShadow(
    ctx, rawUserText, conversationID, assistantMessageID, 5,
)

// Called only after internal/chat validates one exact first-round
// search_memory({}) call. This seam does not invoke v1 or MarkUsed.
toolResult := service.SearchRelevantAfterMemoryToolCall(
    ctx,
    usermemory.HybridMemoryToolSearchInput{
        ConversationID: conversationID,
        AssistantMessageID: assistantMessageID,
        Query: rawUserText,
        ContractVersion: usermemory.HybridMemoryToolContractVersion,
        ContractSHA256: usermemory.HybridMemoryToolContractSHA256,
    },
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

Migration `064` adds the read-only `go_api_runtime`
`memory_authorize_hybrid_rerank` capability. It reauthorizes the exact pending
RRF surface and current source/scope/revision/hash/epoch/generation state, then
returns only a transient maximum cosine similarity. The query vector and raw
similarity are never persisted. Missing/stale/low admission, invalid or late
rerank output, Provider failure, and post-rerank low scores all fail closed to
an empty hybrid final with zero estimated prompt Memory tokens; v1 chat remains
unchanged and unscored RRF/v1 rows are never copied into the v2 candidate.

The schema-v4 Development policy adds an owner-authorized cloud candidate
judge after that reauthorization. Query and candidate content are
deterministically secret-redacted, labelled only with contiguous request-local
ordinals, and treated as untrusted data. The Provider receives no Memory ID,
revision, scope, authority field, or retrieval score. The fixed prompt accepts
exactly one JSON object containing the fixed schema version and at most five
unique in-range ordinals; an empty array is the only `no_memory` result. BGE
rerank and the judge share one bounded concurrent stage, and either failure,
timeout, malformed output, model/prompt drift, or stale authority produces an
empty hybrid final.

The historical schema-v6 alternative uses `HybridMemoryToolRouter` and
`memory_hybrid_main_model_tool_route_calibration_v1`. `routeHybridMemory`
secret-redacts the current query before the route boundary and verifies the
exact expected model ID, `memory-search-tool-v1` contract version, and contract
SHA-256 on return. The route model receives no candidate content or authority.
No Tool Call records `MEMORY_TOOL_ROUTE_ABSTAINED`; exactly one valid call
releases the unchanged BGE-scored/token-bounded final surface. Provider failure,
cutoff, provenance drift, unknown/duplicate calls, or invalid arguments records
`MEMORY_TOOL_ROUTE_FAILED` and leaves final/tokens empty.

Tool routing starts before query embedding completes so the two independent
Provider stages can overlap under the unchanged hard cutoff. Candidate BGE work
may complete speculatively under the owner egress policy, but its result stays
request-local and is discarded on route abstention/failure. Empty RRF candidates
still await the decision long enough to record `MEMORY_TOOL_ROUTE_EMPTY`,
`ABSTAINED`, or `FAILED` truthfully.

Every started Development route now exposes one replayable completion and is
closed before its capture case finishes, including prepare, embedding,
admission, redaction, and empty-candidate exits. This does not add a retry or
extend the two-second hard cutoff. Capture Recorder writes are separately bound
to the generation that recorded the route input, so a delayed result cannot
attach to the next sequential case.

Schema-v6/profile-v6/cost-basis-v4 are immutable failed `PlanTools` evidence.
Schema-v7 uses policy
`memory_hybrid_main_model_first_tool_round_calibration_v1`; the Development
adapter emits a real first `ToolRoundProvider` request and delegates canonical
Tool definition/hash/validation to `internal/chat`.

Product chat does not install the Development adapter. With
`MEMORY_TOOL_LOOP_ENABLED=true`, `internal/chat` validates the first-round call
and invokes `SearchRelevantAfterMemoryToolCall`. That method never calls the v1
reader or `MarkUsed`, never falls back to v1 or unscored RRF, and requires an
optional `HybridFinalRepository`. After normal prepare/admission/rerank/record,
migration `065` rehydrates only the exact final lane while repeating current
source/settings/epoch/projection/revision/hash/scope/lifecycle/Sensitive
authority. Go verifies ordinal identity and redacts every body again. Any drift,
count mismatch, full redaction, Provider failure, or empty result returns no
Memory. Same-model Tool continuation and recovery remain owned by
`internal/chat`.

The product flag and Worker hybrid flag both default false. Enabling the Tool
Loop requires ready fixed-profile projections; keep
`MEMORY_HYBRID_SHADOW_ENABLED` aligned on the Memory Worker when new or changed
Memory must receive embeddings. No live schema-v7 Development/Validation policy
has authorized rollout, so the deployed default remains the v1 prompt/Usage
path.

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
| `SearchRelevantAfterMemoryToolCall(ctx, input)` | Post-call fixed hybrid retrieval plus migration-065 final hydration; no v1 fallback or Usage mutation. |
| `WithHybridMemoryToolRouter(router)` | Install a Development-only route dependency for an explicit calibration policy. |
| `HybridShadowMemoryFirstToolRoundCalibrationPolicy(modelID)` | Build the exact non-promotional schema-v7 first-ToolRound policy. |
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
hybrid_shadow_repository_postgres.go  migration-059/064 prepare, admission, record, and migration-065 final hydration
deletion_portability.go   deletion JSONL export/authenticate/replay orchestration
action_repository_postgres.go  PR6 action, Activity, Usage, and undo capabilities
lexical_shadow.go        fail-open PR7 orchestration and diagnostic sanitization
lexical_shadow_repository_postgres.go  narrow migration-058 compare capability
hybrid_shadow.go         precision-first embedding/admission/rerank/budget orchestration
hybrid_shadow_repository_postgres.go  narrow migration-059 prepare/record capabilities
hybrid_shadow_admission_postgres.go  narrow migration-064 local admission capability
hybrid_candidate_judge.go  strict ordinal-only cloud candidate-judge contract
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

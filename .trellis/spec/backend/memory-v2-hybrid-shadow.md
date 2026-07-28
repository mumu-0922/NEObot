# Memory v2 hybrid vector shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`059_memory_hybrid_vector_shadow`, Memory BGE-M3 projection/jobs, hybrid
prepare/record capabilities, RRF/rerank/token selection, hybrid diagnostics,
or `MEMORY_HYBRID_SHADOW_ENABLED` wiring.

PR8 keeps the v1 in-process Top 5 as the only prompt and Usage authority. It
adds no reader promotion API, governance frontend, L2/L3, Export/Import, or
Hindsight execution.

## 2. Signatures

The fixed retrieval tuple is:

```text
memory_hybrid_bge_m3_rrf60_v1
siliconflow_bge_m3_v1
Pro/BAAI/bge-m3
1024 dimensions
Pro/BAAI/bge-reranker-v2-m3
RRF k = 60
candidate/final limits = 20/5
target/hard token budgets = 600/900
hard cutoff = 2 seconds
```

Memory Worker capabilities:

```text
memory_worker_claim_embedding_job(UUID, UUID, INTEGER)
memory_worker_hydrate_embedding_job(UUID, UUID, UUID)
memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[])
memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
```

Go API capabilities:

```text
memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
)
memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN,
  INTEGER
)
```

The Go reader seam is:

```go
SearchRelevantWithHybridShadow(
    context.Context, query, conversationID, assistantMessageID string,
    limit int,
) ([]Memory, HybridShadowSummary, error)
```

## 3. Contracts

### Projection and embedding

- `user_memory_search_projections` owns one rebuildable fixed-profile
  `vector(1024)` plus `pending|ready|failed` state and a partial HNSW cosine
  index. Equal dimensions never authorize another model/profile.
- Canonical create/content/revision/hash changes set the vector to pending and
  idempotently replace its derived job in the same transaction. Delete,
  disable, non-active lifecycle, archive, or purge physically removes the
  projection/job.
- Scope, epoch, or projection-generation rebind may reuse a ready vector only
  because content/profile are unchanged. A pending/processing job is rebound
  or removed so its old response cannot cross the new authority.
- Every job pins user, Memory ID, revision, hash, visibility epoch, exact scope
  identity/generation, projection generation, embedding tuple, Provider record,
  and Provider `updated_at`.
- Claim admits exactly one active, enabled, attested `RAG:SILICONFLOW` record
  for the user. Duplicate eligible records fail closed to no claim.
- Provider attestation fingerprints concatenate UTF-8 `BYTEA` values with
  `decode('00','hex')`; PostgreSQL `TEXT` cannot contain `chr(0)` and must not
  be used to emulate NUL-delimited fingerprint bytes.
- Hydrate returns one bounded current Memory body and one encrypted Provider
  reference through the live lease. Complete repeats every job/current
  canonical/scope/time/Sensitive/generation/Provider fence before writing a
  finite, non-zero 1024-dimensional vector.
- The raw canonical body/hash remains the SQL lease authority. Immediately
  before Provider egress, the Worker creates a deterministic secret-redacted
  transient body. Partial redaction embeds only the surviving text; full
  redaction makes zero Provider calls and terminally fails the matching
  projection with bounded `EMBEDDING_SECRET_REDACTED` evidence.
- The Worker has no projection/job/provider/canonical table CRUD. Logs contain
  only job ID, Memory ID, bounded code, and status.
- Reclaiming an expired final lease changes the job to `dead_letter` and its
  still-matching projection to `failed/LEASE_EXPIRED` in the same transaction.
  A terminal job must never leave an unreclaimable false `pending` projection.

### Hybrid retrieval and recording

- The shared flag is default-off and gates both embedding claims and API
  hybrid comparisons. `false` means zero Memory embedding/rerank Provider
  calls. Projection correctness does not depend on the flag.
- The v1 reader always runs first. Hybrid failure never changes its items,
  prompt, Usage links, or chat success.
- Prepare accepts only the authenticated user's current streaming assistant,
  its exact completed user parent, and the current active Conversation/Project.
  Query text is transient and must match its SHA-256 and source message.
- SQL source/hash and lexical checks continue to use that raw query. Query
  embedding and rerank use deterministic secret-redacted transient copies.
  A fully redacted query records `query_embedding_status=redacted`, skips both
  Provider stages, and falls back under bounded `SECRET_REDACTED`; a fully
  redacted candidate document skips rerank and preserves RRF order.
- Exact Top 20, CJK BM25 Top 30, and BGE cosine Top 30 apply user/scope/
  Sensitive/time/epoch/generation/revision/hash/profile fences inside each
  candidate query. Query embedding failure removes only the vector lane.
- Fusion is deterministic `sum(1/(60+lane_rank))`, deduplicated by Memory ID,
  with exact membership/rank and UUID tie-breaks. Raw lane/RRF scores are never
  persisted.
- Only authorized RRF Top 20 content is transiently sent to the fixed BGE
  reranker. Rerank invalid/failure/cutoff uses RRF order; no retry can exceed
  the two-second request-local boundary.
- Provider return is not completion authority. Query embedding, rerank, and
  Worker embedding must inspect their stage context after the Provider returns;
  a Provider that ignores cancellation and returns apparent success after the
  deadline is treated as cutoff/retry and its output is discarded.
- Final selection contains at most five rows and uses a conservative
  multilingual estimator. It records whether the 600 target was exceeded but
  never exceeds 900 estimated tokens.
- Record revalidates the assistant/source, reader generation, user, scope,
  Sensitive switch, validity/expiry, canonical revision/hash, and projection
  for every submitted ID after Provider work. Drift produces `RESULT_STALE`
  and no stale final row.
- Exact assistant/query/ordered-v1/result replay is immutable. Same payload
  returns the first evidence; changed payload or result fails with
  `MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT`.
- Durable observations/results store only hashes, profile/generation, IDs,
  revisions, scopes, lane ordinals, bounded status/fallback/count/token/
  duration values. They store no query, Memory body, prompt, embedding,
  Provider secret/authority, or raw score.

### Privilege and rollback

- `go_api_runtime` receives only hybrid prepare/record EXECUTE.
  `memory_worker_runtime` receives only embedding lease EXECUTE. Neither gets
  direct projection/job/observation CRUD or the other role's functions.
- All functions are `SECURITY DEFINER`, owned by `memory_runtime_owner`, and pin
  the application schema followed by `pg_catalog, pg_temp`.
- Down requires the v1/NULL reader and empty hybrid observation history. A
  clean `058 -> 059 -> 058 -> 059` may discard/rebuild vectors and jobs; never
  delete observation evidence to force rollback.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| PostgreSQL/pgvector/PR7 prerequisite differs | Migration aborts before adding PR8 objects. |
| Shared flag absent/false | No embedding claim, query embedding, or rerank call. |
| Provider missing, disabled, unattested, duplicated, or changed | No claim or bounded fallback/retry; never expose the credential. |
| Provider fingerprint is built through PostgreSQL `TEXT` plus `chr(0)` | Forbidden; build the exact NUL-delimited `BYTEA` sequence instead. |
| Job lease/revision/hash/epoch/scope/generation/profile drifts | Old response cannot complete; retry/dead-letter under bounded code. |
| Final embedding lease expires | Atomically dead-letter the job and fail the matching pending projection with `LEASE_EXPIRED`. |
| Embedding is wrong length, zero, NaN, or infinite | Reject; never mark projection ready. |
| Embedding body is fully secret-redacted | Zero Provider calls; terminal `EMBEDDING_SECRET_REDACTED` projection failure. |
| Query or rerank document is fully secret-redacted | Zero corresponding Provider calls; `SECRET_REDACTED` lexical/RRF fallback. |
| Query embedding fails | Exact/BM25 and chat continue without vector. |
| Vector SQL fails | Exact/BM25 RRF fallback and chat continue. |
| Rerank fails or its reserved deadline expires | Record RRF-order fallback before the hard cutoff when possible. |
| Provider returns success after its context deadline | Discard the late output; never complete/rerank from it. |
| Submitted rerank/final ID is not current authorized RRF authority | `RESULT_STALE`, no stale final row. |
| Final estimate would exceed 900 | Skip that candidate; never exceed the hard budget. |
| Replay changes query, baseline, profile, or result | Conflict; first evidence remains unchanged. |
| Runtime attempts direct table access or cross-role function use | PostgreSQL permission denied. |
| Down sees promoted reader or hybrid history | Guarded refusal; schema remains applied. |

## 5. Good / Base / Bad Cases

- **Good**: exact and BM25 find a Chinese preference while BGE finds a
  paraphrase; RRF produces deterministic Top 20, rerank reorders only current
  authorized content, and a content-free observation records a budgeted final
  list while v1 remains the prompt/Usage source.
- **Base**: the flag is false or query embedding is unavailable. Canonical
  projection/jobs stay correct, chat uses v1, and no Memory Provider call is
  made when false.
- **Bad**: claim with an arbitrary RAG record, reuse an old vector response
  after epoch/scope drift, rank cross-user then filter in Go, persist query or
  raw scores, or inject Hybrid final IDs before a separate promotion decision.

## 6. Tests Required

- Go: default-off zero calls, fixed vector shape, capture/job fence drift,
  Provider and hydration retry classification, query/document/body redaction
  and secret-only zero-egress, query-embedding fallback, deterministic rerank
  validation, reserved cutoff recording, 600/900 token selection, bounded
  metadata, and byte-equivalent v1 prompt/Usage behavior.
- Static migration: fixed BGE tuple/HNSW, full job authority shape, no durable
  private payload/raw scores, three independent lanes, RRF(60), current record
  reauthorization, exact grants, and both down guards.
- PostgreSQL 17: full replay/backfill, fake 1024d claim/hydrate/complete/retry,
  final-lease projection failure, scope/epoch/provider old-response fences,
  duplicate-Provider fail-closed, exact/BM25/vector independence, RRF
  determinism, fallback, result stale, replay conflict, role denial, guarded
  down, clean down, and re-up.
- Run focused race, all backend tests, `go vet ./...`, Compose/preflight,
  backend image build, and the full standalone gate. No test calls a Live
  Provider or touches Live user Memory.

## 7. Wrong vs Correct

### Wrong

```text
claim any configured embedding provider
  -> store vector without epoch/scope/provider fence
  -> accept Provider success returned after its deadline
  -> mix BM25/cosine raw scores linearly
  -> rerank stale content
  -> write query/scores to diagnostics
  -> inject Hybrid Top 5
```

### Correct

```text
shared default-off flag
  -> exact attested BGE job lease + current authority completion
  -> post-return deadline rejection + terminal lease/projection closure
  -> independently authorized exact/BM25/vector lanes
  -> deterministic RRF(60)
  -> fixed BGE rerank with reserved record deadline
  -> current-authority revalidation + 600/900 budget
  -> content-free observation
  -> unchanged v1 prompt and Usage
```

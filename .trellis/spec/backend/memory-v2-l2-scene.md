# Memory v2 L2 Scene contracts

## 1. Scope / Trigger

Apply this contract when changing migration `062_memory_l2_scene`, L2 Scene
refresh/embedding/purge jobs, derived Scene retrieval, L2 promotion/rollback,
Scene governance APIs/UI, or either `MEMORY_L2_SCENE_*` runtime flag.

L2 Scene is rebuildable derived data over current canonical L1. It never owns
a user fact and never permits direct plaintext editing. PR11 does not implement
L3 Persona or Hindsight and must ship with the L2 profile in `shadow` and both
runtime flags disabled because no formal promotion evidence currently exists.

## 2. Signatures

Fixed profiles and budgets:

```text
memory_l2_scene_synthesis_v1
memory_l2_scene_hybrid_bge_m3_rrf60_v1
siliconflow_bge_m3_v1 / Pro/BAAI/bge-m3 / 1024 dimensions
Pro/BAAI/bge-reranker-v2-m3 / RRF k=60
candidate/final limits = 20/2
L2 hard prompt budget = 500 estimated tokens
hard cutoff = 2 seconds
```

Runtime gates:

```text
MEMORY_L2_SCENE_SHADOW_ENABLED=false
MEMORY_L2_SCENE_READER_ENABLED=false
```

Worker capabilities:

```text
memory_worker_claim_l2_scene_job(UUID, UUID, INTEGER, BOOLEAN)
memory_worker_hydrate_l2_scene_refresh(UUID, UUID, UUID)
memory_worker_complete_l2_scene_refresh(UUID, UUID, UUID, JSONB)
memory_worker_complete_l2_scene_purge(UUID, UUID, UUID)
memory_worker_retry_l2_scene_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
memory_worker_claim_l2_scene_embedding_job(UUID, UUID, INTEGER)
memory_worker_hydrate_l2_scene_embedding_job(UUID, UUID, UUID)
memory_worker_complete_l2_scene_embedding_job(
  UUID, UUID, UUID, REAL[]
)
memory_worker_retry_l2_scene_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
```

Authenticated Go capabilities:

```text
memory_l2_scene_reader_authority(UUID, UUID, UUID, BOOLEAN)
memory_prepare_l2_scene_search(
  UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN
)
memory_record_l2_scene_search(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER
)
memory_governance_l2_scene_detail(UUID, UUID)
memory_governance_set_l2_scene_enabled(UUID, UUID, BIGINT, BOOLEAN)
memory_governance_rebuild_l2_scene(UUID, UUID)
memory_governance_rebuild_l2_scenes(UUID)
```

Migration-owner-only capabilities:

```text
memory_operator_promote_l2_scene(UUID, JSONB, JSONB)
memory_operator_rollback_l2_scene(UUID, TEXT)
```

## 3. Contracts

### Derived authority and lifecycle

- A Scene is `global` or `project`; Conversation L1 is never widened. Every
  member row binds the same authenticated user and exact current L1 ID,
  revision, content hash, scope identity/generation, and visibility epoch.
- Scene content, topic key, sensitivity, member list, source watermark,
  synthesis profile, L2 generation, and revision are one atomic apply. The
  Provider proposes only `topicKey`, `content`, and member IDs; Go and SQL
  recompute every authority field and reject the whole batch on drift.
- Provider output is strict versioned JSON, at most eight Scenes, two through
  twenty unique input members per Scene, bounded UTF-8 content, and no unknown,
  duplicate, cross-scope, or secret fields. Topic keys identify rebuildable
  projections, not facts or ownership.
- `shadow|active|disabled|stale` describes reader eligibility, not canonical
  truth. Background refresh preserves an explicit user-disabled Scene. Rebuild
  never silently enables it, and no API directly patches Scene plaintext.
- Canonical L1 insert/update/move/disable/supersede/delete, Project lifecycle or
  generation, visibility epoch, and Sensitive policy changes invalidate the
  affected Scene in the database transaction, remove its projection, advance
  the independent L2 generation, and enqueue rebuild. Conversation-only L1
  changes do not create a wider Scene.
- Read and complete repeat member validity/expiry/revision/hash checks. An
  expired member makes a Scene immediately unreadable even before the worker
  materializes `stale`; claim performs a provider-free expiry sweep.
- Stale derived plaintext, member links, and projection are purged within 24
  hours by a provider-free leased job. Account cascade deletes all L2 rows;
  supported backups may retain encrypted database bytes for at most eight
  weeks under the existing backup-set retention contract.

### Provider privacy and worker fencing

- The shadow flag gates refresh and shadow retrieval Provider work. `false`
  means refresh jobs remain pending and no Scene synthesis/query-embedding/
  rerank Provider call occurs. Provider-free stale detection and purge remain
  enabled so disabling shadow cannot weaken deletion.
- Refresh pins user, exact scope, Project generation, visibility epoch, L2
  generation, synthesis profile, all-source watermark, task-model Provider
  record, Provider `updated_at`, lease token, and deadline across claim,
  hydrate, Provider work, and complete.
- SQL hydrates only current active L1 from the exact Scene scope and respects
  the Sensitive switch before plaintext leaves PostgreSQL. Go then applies the
  shared deterministic secret redactor. A fully redacted scope makes zero
  Provider calls; a secret-like Provider result is discarded without durable
  plaintext.
- Scene sensitivity is the stricter of every member sensitivity and the local
  derived-content classifier. The Provider cannot downgrade it. Sensitive
  Scene projection, embedding, search, rerank, and injection each independently
  require the live Sensitive switch.
- Scene embeddings reuse the exact attested BGE-M3 tuple and a separate leased
  derived job. Completion revalidates Scene revision/hash/member watermark/
  generation/profile/Provider authority; late or old responses cannot revive
  stale/deleted content.
- Worker logs contain only job/Scene IDs and bounded codes/status. Runtime
  roles cannot read Scene/member/projection/job/provider tables directly.

### Retrieval, promotion, and governance

- Shadow retrieval and active retrieval share independently authorized Exact
  Top 20, CJK BM25 Top 30, vector Top 30, deterministic RRF(60), fixed BGE
  rerank, and current-authority final validation. No raw score, query, content,
  embedding, Provider identity, or secret is stored in observations.
- Retrieval sees only relevant Global Scenes and Scenes for the current active
  Project. It never loads every navigation row. Final selection is at most two
  Scenes, never exceeds 500 estimated L2 tokens, and fails open to L1 at the
  two-second cutoff.
- Shadow final IDs never enter the prompt or Usage. Active injection requires
  both env reader enablement and database authority: an active L2 profile, user
  `l2_mode != off`, effective Memory Use/Search, current L1 hybrid reader
  pointer, current L2 generation, active Scene, current members, and live
  Sensitive authorization.
- Promotion is an explicit migration-owner transaction separate from the
  evaluator. It requires a strict passing 500-case report with all benchmark
  thresholds and zero leaks, at least seven elapsed days and 100 eligible
  reviewed shadow turns, zero Scene dead letters, and the L1 hybrid pointer.
  A missing or fabricated draft cannot be treated as promotion evidence.
- Promotion evidence validates exact JSON keys by comparing the sorted
  `jsonb_object_keys` result with a lexicographically sorted allowlist. Tests
  must keep synthetic observation timestamps and RFC3339 canary boundaries at
  PostgreSQL `timestamptz` microsecond precision; nanosecond rounding can move
  the first otherwise eligible observation outside an inclusive window.
- Promotion and rollback append immutable bounded events. Rollback changes only
  L2 reader authority and active Scene lifecycle; it does not delete canonical
  L1, change L3 generation, or prevent L1 chat fallback.
- Governance snapshot exposes bounded profile/generation/status metadata and
  Scene summaries. Detail hydrates member content/evidence only from current
  authorized L1 and otherwise returns source-deleted/unavailable markers.
  Scene correction uses existing governed L1 create/update and the same
  invalidation/rebuild chain.

### Privilege and rollback

- `go_api_runtime` receives only Scene search and governance EXECUTE.
  `memory_worker_runtime` receives only Scene refresh/purge/embedding lease
  EXECUTE. Promotion functions remain migration-owner-only.
- Every application capability is `SECURITY DEFINER`, owned by
  `memory_runtime_owner`, and pins the application schema followed by
  `pg_catalog, pg_temp`.
- Down refuses any non-shadow promotion event, Scene/search observation,
  non-empty derived content/history, or active L2 reader pointer. A clean
  `061 -> 062 -> 061 -> 062` discards only empty/rebuildable PR11 state.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| PostgreSQL/pgvector/BM25/PR8 prerequisite differs | Migration aborts before adding PR11 objects. |
| Shadow flag false | Zero refresh/query-embedding/rerank calls; provider-free purge still runs. |
| Conversation L1 is proposed as Scene member | Reject the complete batch; never widen scope. |
| Member ID is absent from hydrated input or revision/hash drifts | Reject/terminally stale the old job; no partial Scene apply. |
| Provider output is unknown/duplicate/oversized/secret | Reject the complete batch and persist no returned plaintext. |
| Sensitive switch turns off during Provider work | Complete fails stale; no Scene/projection survives. |
| Provider returns after its deadline | Discard the apparent success and retry under a bounded code. |
| L1 is updated/deleted/expires after Scene build | Scene is immediately excluded; stale/purge/rebuild follows. |
| User disabled an existing topic | Refresh may update derived bytes but must preserve `disabled`. |
| Query embedding or rerank fails | Use Exact/BM25 or RRF fallback; L1/chat continues. |
| Final Scene would exceed 500 tokens | Skip it; never exceed the L2 hard budget. |
| Active env flag but database profile/L1 pointer/gates are absent | No L2 injection and no claim of promotion. |
| Benchmark/canary/leak/dead-letter gate fails | Promotion transaction aborts without changing reader authority. |
| Runtime attempts promotion or direct table CRUD | PostgreSQL permission denied. |
| Down sees promotion/history/derived state | Guarded refusal; schema remains applied. |

## 5. Good / Base / Bad Cases

- **Good**: two current Project L1 rows produce one strict shadow Scene. Member
  hashes and watermark remain current, hybrid search records only IDs and
  bounded telemetry, and a later L1 correction stales it before the old
  Provider response can complete.
- **Base**: flags are false or no formal promotion exists. Rebuild requests can
  queue, purge still executes, UI shows `shadow/off`, and v1 L1 remains the
  only prompt/Usage authority with zero Scene Provider calls.
- **Bad**: summarize Conversation L1 into a Project Scene, trust Provider
  sensitivity/member IDs, leave a deleted member readable until refresh,
  silently re-enable a disabled topic, inject all navigation rows, or make a
  passing evaluator mutate the active reader.

## 6. Tests Required

- Go Worker: strict JSON keys/counts/topic/member subset, local sensitivity,
  secret-only zero-egress, late Provider return, lease/profile/watermark drift,
  atomic multi-Scene apply, disabled preservation, retry/dead-letter, and
  provider-free purge while shadow is disabled.
- Go reader/API: shadow zero injection, active authority preflight, exact/BM25
  and embedding/rerank fallback, current final reauthorization, 500-token
  selection, L1 fail-open, typed Scene detail/status/rebuild validation, and
  bounded metadata.
- Static migration: complete derived authority shape, same-scope member FKs,
  mutation/setting/project/epoch invalidation, 24-hour purge, fixed retrieval
  tuple, zero-content observations, exact grants, owner/search path, sorted
  promotion-evidence key allowlists, thresholds, and down guards.
- PostgreSQL 17: `061 -> 062 -> 061 -> 062`, claim/hydrate/apply/reclaim,
  cross-user and cross-scope denial, revision/hash/delete/expiry/Sensitive old
  response fences, derived embedding completion, stale/purge, active search,
  promotion denial/success/rollback with microsecond-aligned canary boundaries,
  runtime role denial, and account cascade.
- Frontend: server-only Scene composition, profile/status/member/evidence
  rendering, disable/enable/rebuild/correction-through-L1, stale/error/empty
  states, keyboard names, and no local-mode or direct-derived mutation path.
- Run focused race, all backend tests/vet, all frontend checks/build, RAG full,
  Compose/preflight/backend image, security/change/quality gates, and
  `verify-standalone.sh --full`. Tests use only fake Providers and disposable
  PostgreSQL; they never read or change Live Memory.

## 7. Wrong vs Correct

### Wrong

```text
read every Memory row in Go
  -> ask a Provider for an unbounded summary
  -> trust returned IDs/sensitivity
  -> overwrite one editable Scene
  -> inject all Scenes every turn
  -> delete the L1 fact but wait for the next rebuild
```

### Correct

```text
default-off independent Scene lane
  -> same-scope current L1 + secret/Sensitive pre-egress gate
  -> lease/profile/generation/watermark-pinned strict proposal
  -> SQL reauthorization + atomic derived/member/projection apply
  -> immediate stale on every L1 authority change + 24h purge
  -> relevant-only hybrid shadow observations
  -> explicit benchmark/canary promotion or independent rollback
  -> governed L1 correction and rebuild, never direct Scene authority
```

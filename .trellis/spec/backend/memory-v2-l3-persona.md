# Memory v2 L3 Persona contracts

## 1. Scope / Trigger

Apply this contract when changing migration `063_memory_l3_persona`, L3
Persona refresh/embedding/purge jobs, Persona retrieval, L3 promotion or
rollback, Persona governance APIs/UI, or either `MEMORY_L3_PERSONA_*` runtime
flag.

L3 Persona is one rebuildable derived profile over a user's current eligible
Global L1 Memory. It is never canonical truth, never edits L1, never widens
Project or Conversation scope, and never depends on or mutates the independent
L2 Scene pointer or generation. PR12 ships the L3 profile in `shadow` with both
runtime flags disabled because no formal promotion evidence exists.

## 2. Signatures

Fixed profiles and budgets:

```text
memory_l3_persona_v1
memory_l3_persona_synthesis_v1
memory_l3_persona_hybrid_bge_m3_rrf60_v1
siliconflow_bge_m3_v1 / Pro/BAAI/bge-m3 / 1024 dimensions
Pro/BAAI/bge-reranker-v2-m3 / RRF k=60
candidate/final limits = 5/1
L3 hard prompt budget = 300 estimated tokens
hard cutoff = 2 seconds
```

Runtime gates:

```text
MEMORY_L3_PERSONA_SHADOW_ENABLED=false
MEMORY_L3_PERSONA_READER_ENABLED=false
```

Worker capabilities:

```text
memory_worker_claim_l3_persona_job(UUID, UUID, INTEGER, BOOLEAN)
memory_worker_hydrate_l3_persona_refresh(UUID, UUID, UUID)
memory_worker_complete_l3_persona_refresh(UUID, UUID, UUID, JSONB)
memory_worker_complete_l3_persona_purge(UUID, UUID, UUID)
memory_worker_retry_l3_persona_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
memory_worker_claim_l3_persona_embedding_job(UUID, UUID, INTEGER)
memory_worker_hydrate_l3_persona_embedding_job(UUID, UUID, UUID)
memory_worker_complete_l3_persona_embedding_job(
  UUID, UUID, UUID, REAL[]
)
memory_worker_retry_l3_persona_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
```

Authenticated Go capabilities:

```text
memory_l3_persona_reader_authority(UUID, UUID, UUID, BOOLEAN)
memory_prepare_l3_persona_search(
  UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN
)
memory_record_l3_persona_search(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER
)
memory_governance_l3_persona_detail(UUID, UUID)
memory_governance_set_l3_persona_enabled(UUID, UUID, BIGINT, BOOLEAN)
memory_governance_rebuild_l3_persona(UUID, UUID)
memory_governance_rebuild_l3_personas(UUID)
```

Migration-owner-only capabilities:

```text
memory_operator_promote_l3_persona(UUID, JSONB, JSONB)
memory_operator_rollback_l3_persona(UUID, TEXT)
```

## 3. Contracts

### Derived authority and lifecycle

- Persona input is current active Global L1 owned by the authenticated user.
  Only `fact|preference|instruction|warning|decision` is eligible. Project and
  Conversation rows, `project|context`, disabled/superseded/deleted/expired
  rows, wrong epochs, and Sensitive rows disallowed by current policy are not
  hydrated or sent.
- Every member pins user, Memory ID, revision, content hash, visibility epoch,
  L3 generation, and the all-source watermark. The Provider may propose only
  `content` and two through fifty unique member IDs from that exact hydrated
  set; Go and SQL recompute content hash, sensitivity, token count, lifecycle,
  member authority, and projection fields.
- Provider output is strict JSON containing exactly one Persona. Unknown or
  duplicate fields/members, outside-authority IDs, malformed UUIDs, secret
  output, more than fifty members, fewer than two members, over 4,000
  characters, or more than 300 estimated tokens fail closed without persisting
  returned plaintext.
- `shadow|active|disabled|stale` is derived-reader state, not truth. A user
  disable removes the search projection. Refresh and rebuild preserve the
  latest explicit disable; no API patches Persona plaintext or silently
  re-enables it.
- Any eligibility-changing Global L1 insert/update/move/disable/supersede/
  delete, visibility epoch change, or Sensitive-policy change advances the
  independent L3 generation, stales old Persona, removes its projection, and
  enqueues refresh in database authority. Project/Conversation-only changes do
  not become Persona input.
- Read, hydrate, embedding completion, and final record all repeat current
  member revision/hash/scope/type/epoch/expiry/Sensitive checks. Expired or
  changed members make Persona unreadable before asynchronous stale materialization.
- Stale Persona plaintext, members, and projection are purged within 24 hours
  by a provider-free leased job. Account deletion cascades all per-user L3
  rows without attempting to recreate deleted user state. Existing encrypted
  backup retention remains at most eight weeks.

### Provider privacy and worker fencing

- `MEMORY_L3_PERSONA_SHADOW_ENABLED=false` blocks Persona refresh, query
  embedding, rerank, and derived embedding Provider work. Provider-free stale
  detection and purge remain claimable while the flag is false.
- Refresh pins user, visibility epoch, L3 generation, Persona profile, source
  watermark, task-model Provider record, Provider `updated_at`, model, lease,
  and deadline across claim, hydrate, Provider work, and complete. Lease
  reclaim, generation change, policy change, Provider edit, or late return
  makes the old response unusable.
- SQL filters scope/type/lifecycle/expiry/Sensitive authority before plaintext
  leaves PostgreSQL. Go applies the shared deterministic secret redactor again.
  Fully redacted input or fewer than two authorized members makes zero
  synthesis calls.
- Persona sensitivity is the stricter of member sensitivity and the local
  derived-content classifier. The Provider cannot downgrade it. Turning
  Sensitive off stales the prior generation immediately; no old completion,
  projection, embedding, search result, or prompt injection may survive.
- Derived embedding reuses only the attested SiliconFlow BGE-M3 profile. The
  worker and API composition roots must bind that profile whenever the L3
  shadow lane is enabled, even if the L1/L2 shadow flags are disabled.
- Durable observations contain IDs, hashes, ordinals, bounded counts/status,
  token estimate, and duration only. They never contain query text, Persona
  content, vectors, raw scores, Provider records, or credentials. Logs keep
  only bounded codes and job/Persona IDs.

### Retrieval, promotion, and governance

- Persona search uses separately authorized Exact/CJK BM25/vector candidate
  lanes, deterministic RRF(60), and fixed BGE rerank. Query-embedding failure
  degrades to lexical lanes; rerank failure degrades to RRF. Any L3 failure
  leaves L1, L2, and chat behavior unchanged.
- Candidate count is at most five and final count at most one. Final selection
  is recomputed from authoritative Persona token counts and never exceeds 300
  estimated tokens. Provider results are reauthorized after rerank.
- Shadow mode never returns Persona plaintext to chat and never creates L1
  Usage. Active injection requires the API reader flag plus an active database
  profile, `l3_mode != off`, effective Memory Use/Search, the current L1 hybrid
  pointer, current L3 generation, active Persona, current members, and current
  Sensitive authorization.
- Active content uses its own lower-priority `<relevant-user-persona>` prompt
  block. Only content is sent; Persona/member IDs are omitted. The current user
  request and atomic L1 Memory remain higher authority.
- Promotion is a migration-owner transaction independent of evaluation. It
  requires a strict passing 500-case report, Persona consistency at least
  `0.95`, false injection at most `0.02`, token saving ratio at least `0.20`,
  zero cross-user/secret/delete/provider leaks, at least seven days and 100
  eligible shadow turns, zero dead letters, and the current L1 hybrid pointer.
- Promotion and rollback append immutable bounded events and change only L3
  authority. Rollback does not change L1, L2 generation/pointer, canonical
  Memory, or chat fallback.
- Governance snapshot exposes L3 profile/status/generation and the current
  Persona lifecycle/content/token/sensitivity/watermark/member metadata.
  Detail hydrates only current member L1/evidence; changed or deleted sources
  return markers without reconstructing old plaintext. Correction goes through
  governed L1 followed by rebuild.

### Privilege and rollback

- `go_api_runtime` receives only Persona search and governance EXECUTE.
  `memory_worker_runtime` receives only Persona refresh/purge/embedding lease
  EXECUTE. Neither runtime role receives Persona table CRUD or promotion/
  rollback authority.
- Every application capability is `SECURITY DEFINER`, owned by
  `memory_runtime_owner`, with the application schema followed by
  `pg_catalog, pg_temp` pinned in `search_path`.
- Down refuses non-shadow promotion state/history, any L3 observation, or
  non-empty derived content. A clean `062 -> 063 -> 062 -> 063` removes only
  empty/rebuildable PR12 state and leaves migrations `001` through `062`
  byte-identical.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| PostgreSQL/pgvector/BM25/PR8/PR9/PR11 prerequisite differs | Migration aborts before adding PR12 objects. |
| Shadow flag false | Zero L3 Provider work; provider-free purge still runs. |
| Project/Conversation/unstable/secret L1 is proposed | Exclude before Provider and reject if returned as a member. |
| Member revision/hash/watermark/generation drifts | Reject the complete/embedding/search result; no partial apply. |
| Sensitive turns off during Provider work | Old response fails stale and old Persona remains unreadable. |
| Lease expires and another worker reclaims | Old lease token cannot complete or resurrect Persona. |
| User disabled Persona before refresh | New generation remains disabled and has no projection. |
| Query embedding or rerank fails | Lexical or RRF fallback; L1/L2/chat continues. |
| Client spoofs final token estimate | SQL recomputes and rejects the result. |
| Active env flag lacks database authority | No Persona injection and no promotion claim. |
| Runtime attempts promotion or direct table CRUD | PostgreSQL permission denied. |
| Account is deleted | All per-user L3 rows cascade; no state recreation or FK failure. |
| Down sees promotion/history/derived state | Guarded refusal; schema remains applied. |

## 5. Good / Base / Bad Cases

- **Good**: current Global stable L1 produces one strict shadow Persona. Its
  member revisions and watermark remain current, retrieval records only
  content-free diagnostics, and a later L1 or Sensitive change invalidates it
  before an old Provider response can complete.
- **Base**: flags remain false or no promotion evidence exists. Refresh stays
  queued, provider-free purge continues, governance shows `shadow/off`, and L1
  remains the only default prompt/Usage authority with zero L3 Provider calls.
- **Bad**: summarize Project/Conversation rows, trust Provider IDs/sensitivity,
  persist an oversized Persona, return shadow plaintext, inject IDs, silently
  re-enable a disabled Persona, let an old lease complete, or let the evaluator
  mutate reader authority.

## 6. Tests Required

- Go Worker: strict single-Persona JSON, stable type/member subset, duplicate/
  unknown/oversized/secret rejection, secret-only zero-egress, Provider
  deadline, lease/profile/watermark drift, derived embedding, retry/dead-letter,
  and provider-free purge with shadow disabled.
- Go reader/API: shadow zero injection, active authority preflight, lexical/
  vector/rerank fallback, final reauthorization, 5/1/300 bounds, independent
  prompt block without IDs, L1/L2 fail-open, and typed governance routes.
- Static migration: Global/stable member authority, generation invalidation,
  Sensitive fencing, 24-hour purge, fixed profiles, content-free diagnostics,
  least privilege, evidence gates, account-cascade guard, and down guards.
- PostgreSQL 17: `062 -> 063 -> 062 -> 063`, claim/hydrate/apply/reclaim,
  scope/type/member spoof, Sensitive on-to-off old response, embedding,
  Exact/BM25/vector/RRF and lexical fallback, token spoof, disabled preservation,
  stale/purge, promotion denial/success/rollback and L2 independence, runtime
  role denial, cross-user denial, and account cascade.
- Frontend: server-only Persona composition, profile/status/member/evidence
  rendering, disable/enable/rebuild, correction-through-L1, stale/error/empty
  states, accessibility, and no direct-derived plaintext mutation.
- Run focused race, all backend tests/vet, all frontend checks/build, RAG full,
  Compose/preflight/backend image, security/change/quality gates, and
  `verify-standalone.sh --full`. Use only fake Providers and disposable
  PostgreSQL; never read or mutate Live Memory.

## 7. Wrong vs Correct

### Wrong

```text
read every scope into one profile
  -> send raw secrets to an unbounded Provider call
  -> trust returned members and sensitivity
  -> expose/edit Persona as canonical Memory
  -> inject shadow content and IDs every turn
  -> delete L1 but wait for a future refresh
```

### Correct

```text
default-off independent Persona lane
  -> current stable Global L1 + secret/Sensitive pre-egress gate
  -> lease/profile/generation/watermark-pinned strict proposal
  -> SQL reauthorization + derived/member/projection apply
  -> immediate stale on authority change + provider-free 24h purge
  -> relevant-only hybrid shadow diagnostics
  -> explicit evidence-gated promotion or independent rollback
  -> governed L1 correction and rebuild, never Persona authority
```

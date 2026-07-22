# G18.5A retrieval profile pointer design

## Goal and non-goals

G18.5A inserts a reversible, server-owned decision point before changing the
storage engine. The Go repository calls one stable profiled function; its
initial implementation delegates only to the current `ts_rank + REAL[]`
reader. This makes the application change safe on the running PostgreSQL 16
service before the PG17 restore and storage migration.

This stage does not activate BM25/pgvector, change Compose, touch the live data
directory, expose an admin/browser setting, or consume provider quota. G18.5B
will add the PG17 implementation and replace the guarded unavailable branch.

## State machine

```text
migration 037 on PG16
       |
       v
legacy @ revision 1 ---- profiled reader ----> existing hybrid function
       |
       +-- target pg17 -> RAG_RETRIEVAL_PROFILE_UNAVAILABLE

migration 038 on fresh PG17 (future)
       |
       +-- verified backfill + operations gates
       v
pg17_bm25_pgvector_v1 @ revision N
       |
       +-- compare-and-swap rollback -> legacy @ revision N+1
```

`knowledge_retrieval_profile_head` is a singleton with a monotonic revision.
`knowledge_set_retrieval_profile(expected, target, revision, reason)` locks the
row under an advisory transaction lock and rejects stale callers with SQLSTATE
`40001`. Successful changes append an immutable transition record. A same-state
request is idempotent and does not advance the revision.

## Reader contract

`knowledge_fetch_profiled_query_evidence_candidates(...)` preserves the legacy
function signature and reference-only result shape. The Go repository changes
only the database function name. On profile `legacy`, the router delegates to
`knowledge_fetch_hybrid_query_evidence_candidates(...)`; every input validation,
authority filter, rank, and error therefore remains unchanged.

The PG17 branch intentionally raises `RAG_RETRIEVAL_PROFILE_UNAVAILABLE` in
migration 037. It is safer to fail closed than to permit a pointer value whose
schema, backfill, or indexes are absent.

## Privileges and rollback

Both pointer functions are SECURITY DEFINER with a hardened migration-captured
search path. `go_api_runtime` and `rag_worker_executor` receive EXECUTE only on
the candidate reader. `rag_replay_operator` receives EXECUTE only on the
compare-and-swap function. No runtime role receives direct pointer/history
table access.

Migration down first requires `active_profile = 'legacy'`. If a later PG17
profile is active, down fails inside the migration transaction and leaves the
migration row and all functions/tables intact. Operators must switch back to
legacy and roll back the application reader before removing migration 037.

## Verification boundary

The PG16 drill uses only synthetic data and an internal Docker network. It
compares complete result rows, executes the reader under both runtime roles,
executes pointer operations under the replay role, proves unavailable/conflict
errors do not mutate state, forces the rollback guard, switches back through
the controlled function, then performs down/reapply/restart checks.

## Change history

- 2026-07-22: initial PG16-compatible legacy profile pointer and guarded router.

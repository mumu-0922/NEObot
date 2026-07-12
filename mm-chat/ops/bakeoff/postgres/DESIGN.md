# Phase 15.2A ParadeDB bake-off design

## Objectives

This module answers a narrow promotion question: can the pinned PostgreSQL 16
ParadeDB candidate reproduce the Phase 15 hybrid-search invariants within a
single-server 1 GiB/2 CPU envelope? It validates extension identity, Chinese
and mixed-language lexical behavior, both vector storage lanes, exact-versus-
ANN quality, fail-closed ACL filtering, deterministic fusion, data portability,
recovery, and observable resource use.

The governing design remains:

- [Phase 15 accuracy-first RAG design](../../../docs/architecture/phase-15-accuracy-first-rag-design.md)
- [Phase 15 recommended implementation profile](../../../docs/architecture/phase-15-recommended-implementation-profile.md)
- [Phase 15.2A internal Evidence API](../../../docs/contracts/internal-evidence-api.md)
- [Knowledge ACL API](../../../docs/contracts/knowledge-acl-api.md)

This harness supplies evidence to those documents; it does not promote the
candidate or redefine their contracts.

## Choices

- The image is immutable by digest and the SQL rejects extension versions
  other than `pg_search=0.24.2` and `vector=0.8.2`.
- `PDB_TUNE=false` plus explicit `shared_buffers=256MB`,
  `effective_cache_size=512MB`, `work_mem=4MB`, and
  `maintenance_work_mem=64MB` prevents host-based auto-tuning from exceeding
  the 1 GiB cgroup.
- No host port is published. The runner uses `docker compose exec`, a unique
  project name, and a project-scoped named volume.
- The runner waits for ParadeDB's init-complete log marker before its first
  readiness probe. This accounts for the image's temporary bootstrap server
  shutting down and restarting asynchronously.
- Jieba, Lindera Chinese, and `chinese_compatible` each have a typed table and
  one BM25 index. Tokenizers are compared as separate lanes rather than by
  mutating one index in place. A separate GIN-backed Exact Lane validates raw
  identifiers/paths and case-sensitive phrases without calling BM25 exact.
- ACL tests bind explicit immutable authorized-version UUID arrays before
  search. Mutable document pointers are not joined into a pushed BM25 query.
- Exact `vector(1024)` results form the recall reference; HNSW uses strict
  iterative scans for low-selectivity filters. `halfvec(2048)` has an
  independent HNSW index and top-1 test.
- RRF orders each source and the final fusion by an explicit stable `chunk_id`
  tie-breaker.
- Dump/restore creates the destination from `template0`, installs only the two
  pinned extensions, and restores the application schema; owner and ACL
  metadata are suppressed for portability. Excluding unrelated image-preloaded
  extensions also avoids coupling the archive to `pg_cron`'s configured
  database. A graceful restart and forced `SIGKILL` exercise WAL recovery
  against only the disposable project volume.

## Trust and safety boundaries

- All rows and UUIDs are synthetic. No API key, password, user file, production
  snapshot, or project `.env` is read.
- `POSTGRES_HOST_AUTH_METHOD=trust` is acceptable only because the container
  has no published port and lives on a one-run Compose network. It is expressly
  not a production configuration.
- The EXIT/INT/TERM trap invokes `down --volumes --remove-orphans` with the exact
  generated project name and compose file. It never calls `docker system
prune`, removes arbitrary volumes, or names an existing mm-chat service.
- Reports and logical artifacts are generated under `/tmp`; source directories
  remain deterministic and secret-free.
- Search authorization is a caller-provided allowlist. Search ranking is not an
  authorization source, and post-result filtering is not accepted as a fence.

## Known limits

- Synthetic 500-row vectors and 100 lexical rows validate mechanics, not the
  production corpus's relevance, latency tail, index-build duration, or disk
  growth. Promotion still requires the locked corpus/query benchmark and SLOs.
- HNSW recall is hardware/data/parameter dependent. The harness enforces a
  conservative recall@20 floor but does not tune production parameters.
- Crash recovery is a single-container `SIGKILL`, not host power loss, torn
  storage, replica failover, PITR, or backup retention validation.
- CSV export demonstrates explicit ACL scoping, not a complete privacy/export
  product flow. The dump contains synthetic rows only and is not a production
  backup design.
- The harness does not test the Python consumer, Go evidence gateway, mTLS/JWS,
  outbox fencing, reranker, object storage, or end-to-end citation hydration.

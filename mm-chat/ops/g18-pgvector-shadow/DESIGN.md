# G18.3 pgvector shadow projection design

## Goal and non-goals

The module proves that existing valid Jina v4 `REAL[1024]` passage vectors can
be copied into pgvector `vector(1024)` without re-embedding, while preserving
generation/profile identity, current authority, deletion invisibility, and the
legacy production reader.

It does not upgrade the user's running PostgreSQL 16 database, change the
single-server Compose database, call a provider, expose a shadow reader to Go
or the RAG worker, or cut over production retrieval. BM25 and hybrid dual-read
remain G18.4; the database and reader cutover remain G18.5.

## Architecture

```text
current PG17-restored schema + synthetic current authority
       |
       v
knowledge_pgvector_shadow_sources
  published/current/ready Jina v4 REAL[1024] rows only
       |
       v
knowledge_backfill_pgvector_shadow(generation, search profile)
  preflight finite + non-zero -> immutable vector(1024) copy
       |
       +--> exact cosine verification
       +--> HNSW vector_cosine_ops verification
       +--> selected-collection / deletion verification
       |
       v
shadow-only down -> legacy REAL[] rows and reader unchanged
```

## Core contracts

### Source authority view

`knowledge_pgvector_shadow_sources` contains references and vectors but no
document text. A row is eligible only when its search profile and generation
match, its source projection is ready Jina v4/1024, its materialization is the
published document head, and collection/document/version authority is current.

### Immutable shadow table

`knowledge_child_vector_shadow_projections` stores the complete immutable
identity tuple, visibility revisions, source float32 hash, measured norm, and
`vector(1024)`. Foreign keys plus a `BEFORE INSERT` trigger re-read the source
authority view and require exact `vector -> REAL[]` round-trip equality. The
existing immutable-projection trigger rejects update and delete.

### Backfill

The SECURITY DEFINER backfill accepts an explicit generation and search
profile. It rejects incompatible profiles, zero vectors, NaN, and infinities
before insertion. One `INSERT ... SELECT` copies eligible rows, an idempotent
conflict path preserves existing rows, and a postcondition requires every
eligible source identity/hash/vector to have an exact shadow match. Any error
rolls back the statement transaction; no provider endpoint is involved.

Only `rag_replay_operator` receives EXECUTE. `go_api_runtime` and
`rag_worker_executor` receive neither table access nor function execution, so
the current reader cannot accidentally consume the shadow.

Before creating SECURITY DEFINER functions, the DDL pins `search_path` to the
current schema followed by `pg_catalog, pg_temp`; it does not capture the
default `"$user"` path used by a raw psql session.

## Index decisions

- A B-tree scope index covers generation, search profile, collection, and
  deterministic child identity.
- HNSW uses `vector_cosine_ops`, `m=16`, and `ef_construction=64` as a shadow
  candidate, not a production default.
- The drill forces and verifies the HNSW plan and compares its complete
  four-row ordering with exact cosine. This proves mechanics only; production
  `ef_search`, oversampling, ACL low-selectivity strategy, latency, and resource
  budgets still require the frozen Golden Set and larger corpus in G18.4/G18.5.

## PG16 compatibility decision

The shadow DDL intentionally remains outside `backend/migrations` during
G18.3. The independent project still runs `postgres:16-alpine`; embedding a
`VECTOR(1024)` migration now would make ordinary `migrate up` fail before the
blue-green PostgreSQL 17 cutover. The disposable drill therefore applies the
reviewed DDL only to PG17 and proves that `schema_migrations` remains `1–37`.

G18.5 must promote this reviewed DDL into the normal migration sequence only
after a verified production backup is restored into fresh PG17 storage. It
must not point PG17 at `mm-chat/data/postgres` or mark the DDL applied on PG16.

## Security and rollback

- No project env, provider credential, host port, private source text, or
  production data enters the harness.
- Selected-collection checks and current authority prevent cross-collection
  and tombstoned evidence from becoming visible, even while immutable shadow
  rows remain for rollback.
- Direct insert with a forged source hash is rejected by the validation
  trigger. Invalid vector preflight is proven to leave the shadow row count
  unchanged.
- The down script removes only shadow functions, view, table, and indexes.
  Legacy `REAL[]` source rows and
  `knowledge_fetch_hybrid_query_evidence_candidates(...)` survive unchanged.

## Change history

- 2026-07-22: initial G18.3 PG17 shadow/backfill/exact/HNSW/rollback proof.

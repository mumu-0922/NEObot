# PostgreSQL 16 / ParadeDB bake-off

Ephemeral Phase 15 RAG validation for the digest-pinned ParadeDB PostgreSQL 16
image. It covers `pg_search` 0.24.2, `vector` 0.8.2, independent Jieba,
Lindera Chinese, and `chinese_compatible` BM25 lanes, an Exact Keyword/Phrase
Lane, exact and HNSW vector lanes, `vector(1024)`, `halfvec(2048)`, filtered ACL
recall/leakage, deterministic RRF, rollback, export, dump/restore, restart/crash
recovery, and resource limits.

## Quick start

Dependencies: Bash 4+, Docker Engine with the Compose v2 plugin, standard
`grep`/`mktemp`/`wc` utilities, network access for the first image pull, and at
least 1 GiB RAM plus 2 CPUs available to Docker.

Run from anywhere:

```bash
mm-chat/scripts/run-phase15-rag-postgres-bakeoff.sh
```

Successful and failed runs print their report directory. SQL summaries,
resource samples, authorized CSV export, and the custom-format dump are under
`/tmp/mm-chat-phase15-pg-bakeoff.*`.

## Isolation and cleanup

The runner creates a unique Compose project with no published port or fixed
container name. Its named volume is project-scoped and is removed by an EXIT
trap. It never calls global Docker cleanup or addresses the production stack.
Authentication is `trust` only inside this unexposed, disposable Compose
network; no password or token is stored. Reports, dumps, and authorized CSV
exports are written below `/tmp`, never this repository.

If the shell is ungracefully killed, clean up only the project name printed at
startup:

```bash
docker compose -p <printed-project> \
  -f mm-chat/ops/bakeoff/postgres/compose.yml down --volumes
```

**Non-production warning:** this is a destructive, synthetic bake-off fixture,
not a database migration, production topology, backup policy, or extension
upgrade procedure. Never point it at an existing database or reuse its trust
authentication pattern on a published/shared network.

The deterministic baseline contains 100 lexical rows per tokenizer lane, 500
synthetic vectors, and three Exact Lane rows. It proves query/index/recovery
mechanics, not the production tokenizer/dimension winner; that decision still
requires the locked Relevance Set, SLO run, and license approval.

See [`DESIGN.md`](./DESIGN.md) for decisions, limits, and Phase 15.2 links.

BM25 access control is tested with an explicit authorized-version UUID array.
Production callers must resolve authorized immutable version IDs before the
search statement and bind that array into both lexical and vector lanes; do
not derive authorization by joining a mutable document pointer inside the
pushed search query.

Rollback/export posture: writes are transaction-scoped, `pg_dump` uses a
portable custom archive with owner/ACL metadata suppressed at restore, and
the sample CSV export contains only explicitly authorized rows. Rollback is a
deployment action (restore the prior application/search path or restore the
validated archive), not an in-place extension downgrade.

# G18 BM25 and pgvector Retrieval Process

## 2026-07-22 — Intake and architecture lock

The current runtime was traced before changing storage. Migration `026` uses
PostgreSQL FTS `ts_rank` with exact-term, phrase, and bounded CJK-bigram boosts;
it is not BM25. Migration `027` stores Jina v4 1024-dimensional vectors as
`REAL[]`, expands both arrays during cosine calculation, applies a Dense `0.48`
gate for queries of at least eight characters, and fuses Lexical and Dense
ranks through RRF. Go fuses original and rewritten query lanes, hydrates under
current authority, calls `jina-reranker-v3`, and admits scores `>= 0.0`.

The target is PostgreSQL 17 with `pg_textsearch` plus pgvector, kept behind
shadow/dual-read gates. Existing Jina v4 vectors remain the first production
generation. BGE-M3 is deferred to an isolated shadow benchmark because equal
dimensions do not imply a compatible vector space.

Historical live evidence also ruled out blindly raising the global reranker
threshold. An unused candidate scored `0.11554055`, while useful cited
candidates scored `0.09505704`, `0.12736949`, and `0.21525481`. G18.1 therefore
starts with a synthetic versioned Golden Set and deterministic evaluator. Any
policy promotion must retain required cases and reject unrelated negatives;
one scalar score is not treated as a relevance probability.

Next: implement the G18.1 evaluator and fail-closed reranker degradation,
capture the current-engine synthetic baseline, run focused/full Go checks, and
commit G18.1 before touching the database image.

## 2026-07-22 — G18.1 evaluation and strict degradation closure

Added `internal/rageval` plus `cmd/rag-eval`. The closed JSON contracts accept
only synthetic queries, collection/evidence aliases, ranked lane observations,
final evidence identifiers, citation identifiers, no-evidence decisions, and
latency. Unknown fields are rejected, so source text cannot silently enter a
committed observation. The evaluator deterministically reports per-lane recall,
final-context precision, negative false-citation rate, no-evidence accuracy,
nearest-rank P95 latency, case failures, and a promotion result.

The versioned `g18-synthetic-retrieval-v1` fixture covers exact identifiers,
Chinese lexical and semantic questions, a contextual rewrite, two selected
collections, and Chinese/English unrelated negatives. Its current-engine
observation is recorded in `baseline-current-v1.json`; it contains only
synthetic aliases and scores, never live UUIDs, source text, or credentials.

Configured reranking now fails closed before Knowledge publication. Missing
governance, partial configuration, provider failure, or a malformed result
returns no Knowledge evidence, so the normal Model/Web path may continue but
unreranked RRF candidates cannot mint citations. The relevance policy is named
`g18-jina-reranker-v3-golden-v1` and requires a complete finite rerank result.
Its numeric floor intentionally remains `0.0`: historical useful/unused scores
overlap, and the live frozen negative set passes without a blind threshold
increase. The effective policy is the Golden gate plus mandatory rerank,
fail-closed degradation, and the existing terminal answer-marker citation
filter—not an assumption that a Jina score is a probability.

Live disposable baseline:

```text
engine profile                              ts_rank + REAL[] cosine + Jina
Golden cases                                7 (5 relevant / 2 unrelated)
Lexical / Dense / rewrite-Dense recall      1.0 / 1.0 / 1.0
final-context precision                     1.0
negative false-citation rate                0
no-evidence accuracy                        1.0
Knowledge-stage P95                         25.402s
contextual rewrite                          answered / queryRewritten=true / [K1]
cross-collection                            answered / 2 selected / [K1],[K2]
weather / cooking negatives                 no_evidence / no_evidence
```

The cooking negative deliberately produced one low lexical candidate, then
ended as `no_evidence`; this proves candidate recall alone is not citation
authority. The slowest case was contextual query rewrite, not a PostgreSQL
storage cutover, so later BM25/pgvector comparisons must separate provider and
database latency when diagnosing regressions.

Verification:

```text
focused rageval / CLI / chat tests           passed
go vet ./...                                 passed
go test ./...                                passed
recorded baseline evaluator gate             passed
Compose backend rebuild                      passed
backend database/redis/storage readiness     ready / ready / ready
temporary active collections/documents       0 / 0
temporary ready search projections           0
temporary active conversations               0
local live-probe artifacts                    deleted
```

Rollback: reverting the G18.1 code restores the earlier degraded RRF fallback;
there is no schema change. The Golden Set, baseline, and evaluator are inert
offline artifacts. Database cleanup touched only the deleted disposable
synthetic fixtures and left production Knowledge untouched.

Next: commit G18.1 alone, then start G18.2 with a digest-pinned PostgreSQL 17
extension image and disposable PostgreSQL 16 backup/restore drill. Do not point
PostgreSQL 17 at `data/postgres`.

## 2026-07-22 — G18.2 PostgreSQL 17 restore and rollback proof

Added a multi-stage local PostgreSQL image whose build inputs are immutable:

```text
PostgreSQL base  17.10-bookworm
base digest      sha256:4f736ae292687621d4dbe0d499ffd024a36bd2ee7d8ca6f2ccd4c800f047b394
pg_textsearch    1.3.1 / 578ff529894992fb9e67cae4c69424e65c84868e
archive SHA-256  8632f91231251dc3e19395ef6a0d4d158d5f5920ba420691471771418e2a7cc7
pgvector         0.8.5 / 159b79aaad5983fb7459c1e3df2897fbb2d11788
archive SHA-256  9a483fad70ae2e0a50b3dccb6c4b4931d9a07375a1d5815e82b57870448a7d52
```

The final image copies only installed extension artifacts and licenses from
the builder. pgvector CPU-native optimization is disabled to avoid binding the
artifact to the builder CPU. Runtime initialization requires PostgreSQL 17,
preloaded `pg_textsearch 1.3.1`, and pgvector `0.8.5`. Before delegating to the
official entrypoint, the wrapper rejects any existing non-17 `PG_VERSION`; a
synthetic PostgreSQL 16 marker exited `78` and remained unchanged.

The restore harness uses a unique Compose project, internal-only network,
project-scoped volumes, no host port, no project env file, no provider call,
and only synthetic authority/projection rows. It never reads or mounts
`mm-chat/data/postgres`. The exact proof command was:

```bash
./mm-chat/scripts/run-g18-postgres17-restore-drill.sh
```

The final successful report was written to
`/tmp/mm-chat-g18-postgres17.1DHyMN`. The logical backup checksum was verified
before either restore. Decisive output:

```text
PASS PostgreSQL 17 extension smoke
PASS synthetic PG16 authority/projection fixture
PASS PG16 migrations=36 authority=1 objects=2 generation=active projection=ready
PASS PG17 migrations=36 authority=1 objects=2 generation=active projection=ready
PASS PG16 migrations=36 authority=1 objects=2 generation=active projection=ready
disposable_databases=removed
```

The extension smoke created a real BM25 index, executed a BM25 winner query,
and executed a pgvector cosine winner query. The restore checks covered all
`36` current migrations, collection/document/file authority, parser and source
object references, the active generation head, projection readiness, published
materialization, and Parent/Child/Search rows. Running the current migration
CLI after both restores returned `no migrations changed`. The original PG16
source was verified again after both restores and retained the same state.

Iteration fixes captured during the drill:

- corrected Compose build contexts from one directory too high;
- resolved image identity with an explicit `docker build` because
  `docker compose images -q` has no result before container creation;
- passed the expected PostgreSQL major through a session-local setting because
  psql variables do not expand inside a dollar-quoted `DO` block;
- updated the base pin from PostgreSQL 17.9 to the actually installed and
  verified PostgreSQL 17.10 digest instead of accepting header/runtime drift;
- removed random Compose project labels from the reusable image by building it
  outside the project-scoped Compose build.

Local image evidence:

```text
tag       mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5
image id  sha256:f139b5ee69c4742204579a023b55e43bd42cc04382213c5eb8dae764dbef82a1
size      159679854 bytes
```

Rollback uses the preserved logical backup to populate a fresh PostgreSQL 16
database; it never reuses or downgrades PostgreSQL 17 storage. The exit trap
removed the synthetic databases, containers, network, and volumes. A final
label check reported `containers=0` and `volumes=0`.

Next: commit G18.2 alone. G18.3 then adds the generation/profile-bound
`vector(1024)` shadow projection, transactionally reuses only compatible Jina
v4 `REAL[]` vectors, proves exact/HNSW cosine behavior, and leaves the current
reader in production.

## 2026-07-22 — G18.3 pgvector shadow projection

The production compatibility boundary was resolved before writing DDL. The
independent Compose runtime still uses `postgres:16-alpine`; placing a
`VECTOR(1024)` migration into the ordinary embedded set would make current
`migrate up` fail before the PostgreSQL 17 blue-green transition. G18.3
therefore keeps the reviewed shadow DDL in a PG17-only operational module.
G18.5 must promote it into the normal migration sequence only after a verified
production backup is restored into fresh PG17 storage.

The shadow source view admits only reference/vector rows whose search profile
and generation agree, whose model contract is Jina v4/1024, whose source search
projection is ready, whose materialization is the published document head, and
whose collection/document/version authority is current. It contains no source
text.

`knowledge_child_vector_shadow_projections` stores the complete immutable
identity tuple, visibility revisions, source float32 hash, measured norm, and
pgvector `vector(1024)`. A validation trigger reauthorizes every insert against
the source view, requires exact `vector -> REAL[]` round-trip equality, and
rejects forged identity/hash data. The existing immutable trigger rejects
update/delete. `go_api_runtime` and `rag_worker_executor` receive no shadow
privileges; only `rag_replay_operator` can invoke the SECURITY DEFINER backfill.

The backfill takes an explicit generation and search profile, preflights every
eligible source vector for 1024 dimensions, finite components, and non-zero
norm, inserts from one snapshot statement, and verifies every eligible
identity/hash/vector after insertion. Any failure rolls back the statement.
`ON CONFLICT DO NOTHING` makes a verified replay idempotent. No embedding
provider is called and no Jina quota is consumed.

The disposable proof command was:

```bash
./mm-chat/scripts/run-g18-pgvector-shadow-drill.sh
```

Final report: `/tmp/mm-chat-g18-pgvector-shadow.wKfXMS`.

```text
PASS G18.3 pgvector shadow schema
PASS G18.3 synthetic pgvector fixture rows=4 collections=2
PASS G18.3 golden=7 backfill=4 exact/hnsw=parity acl=2 deletion=hidden invalid=rollback
PASS G18.3 pgvector shadow rollback
PASS G18.3 rollback retained legacy REAL[] reader/data
no migrations changed
disposable_database=removed
```

The seven storage cases reuse the frozen G18 case identifiers: four relevant
single-collection cases, cross-collection retention, and two unrelated
no-evidence cases. Exact and HNSW queries returned the approved deterministic
child order at the existing Dense `0.48` gate. `EXPLAIN` proved an actual
`Index Scan using idx_knowledge_child_vector_shadow_hnsw`, rather than a result
that happened to come from a sequential scan.

The versioned G18.1 evaluator was rerun against the recorded current-reader
baseline after the shadow work: all seven cases passed, required lane recall
and final-context precision remained `1.0`, negative false-citation rate
remained `0`, and no-evidence accuracy remained `1.0`.

Negative proofs added zero and NaN `REAL[]` candidates one variable at a time.
Each backfill returned `RAG_PGVECTOR_SHADOW_SOURCE_INVALID`, inserted no partial
row, and preserved the previous shadow count. A direct insert with a forged
float32 hash returned `RAG_PGVECTOR_SHADOW_SOURCE_MISMATCH`. After tombstoning
the first document, its three immutable shadow rows remained available for
rollback but the authority view and legacy production reader both returned no
candidate immediately; the separately selected collection remained isolated.

The down script removed only the shadow table, indexes, view, validation
function, and backfill function. Six legacy `REAL[]` rows (four valid plus two
purged negative fixtures) and
`knowledge_fetch_hybrid_query_evidence_candidates(...)` remained. Re-running
the production migration CLI returned `no migrations changed`, proving the
manifest still ends at `036` and current PG16 operation is not broken.

Security review found and closed one psql-specific edge before the final run:
raw psql would otherwise let `SET search_path FROM CURRENT` capture the default
`"$user", public` path. The up/down DDL now pins the current schema followed by
`pg_catalog, pg_temp`, and the live contract inspects both SECURITY DEFINER
functions to reject `$user` or missing catalog/temp entries.

Final quality gates:

```text
bash syntax / Compose config                    passed
module completeness / placeholder / secret     passed
SECURITY DEFINER paths / forbidden grants       2 hardened / 0
go vet ./...                                    passed
go test ./...                                   passed
recorded seven-case evaluator                   passed
temporary containers / volumes                  0 / 0
```

Next: commit G18.3 alone. G18.4 adds true `pg_textsearch` BM25, compares BM25
and pgvector lanes through deterministic RRF against the frozen evaluator, and
keeps all dual-read diagnostics reference-only and server-owned.

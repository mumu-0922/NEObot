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

## 2026-07-22 — G18.4 BM25 and hybrid dual-read shadow

G18.4 stayed on the PG17-only operational boundary established in G18.3. The
independent project still runs PostgreSQL 16, so no `pg_textsearch` or `VECTOR`
DDL was added to `backend/migrations`; the production migration manifest
remained `1–36` and the `ts_rank + REAL[]` reader remained the only Go reader.

A live extension probe first established the `pg_textsearch 1.3.1` contract:
`<@>` returns negative BM25 scores, lower is better, and no match is `0`.
`to_bm25query(query, index_name)` produced a real BM25 index scan. The shadow
reader therefore filters `score < 0`, preserves raw scores only as diagnostics,
and never treats them as probabilities.

The new `knowledge_bm25_shadow_sources` view admits only rows under the active
corpus head, active generation, ready Jina v4/1024 search profile, active
projection head, published materialization, and current collection/document/
version visibility. The verified, idempotent backfill derives a BM25 document
from lexical text, normalized exact terms, and at most 512 CJK ideograph
bigrams. A trigger rejects forged identity or derived text. BM25 rows remain
immutable rollback artifacts, but every read rejoins current authority, so a
tombstone hides them immediately.

One prototype was rejected during the first negative proof: generating bigrams
for all compacted text caused an unrelated English query to share fragments
such as `er` with identifiers and policy text. Bigrams are now generated only
for Chinese ideographs; Latin text remains under the `simple` tokenizer. A
second correction aligned `context-follow-up` with runtime order: the storage
case consumes the standalone rewrite, while a separate proof fuses the raw
follow-up and rewrite result lanes exactly as the Go assembler does. A third
iteration removed a common word from a temporary negative query and restored
the frozen G18.1 weather/cooking prompts.

`knowledge_fetch_hybrid_shadow_diagnostics(...)` validates the selected
collections, query, 1024-dimensional finite/non-zero vector, and result limit.
It retrieves a bounded 8x BM25 pool plus exact terms, retrieves Dense candidates
from the G18.3 pgvector projection at the existing `0.48` gate, assigns stable
per-lane ranks, and fuses them with `1 / (60 + rank)`. It returns UUIDs, hashes,
BM25/Dense ranks and scores, and final rank/score only. Neither source text nor
exact terms are output. Only `rag_replay_operator` can execute it; the live
proof also switched to that role and replayed both backfill and diagnostics
successfully. `go_api_runtime` and `rag_worker_executor` have no shadow
privileges.

The exact disposable proof command was:

```bash
./mm-chat/scripts/run-g18-hybrid-shadow-drill.sh
```

Final report: `/tmp/mm-chat-g18-hybrid-shadow.B7rYSd`.

```text
PASS G18.4 BM25 hybrid shadow schema
PASS G18.4 synthetic hybrid fixture rows=4 collections=2
Index Scan using idx_knowledge_child_bm25_shadow_text
Index Scan using idx_knowledge_child_vector_shadow_hnsw
PASS G18.4 golden=7 bm25+dense=rrf identifiers/cjk=recall negatives=0 deletion=hidden diagnostics=redacted
PASS G18.4 rollback retained G18.3 vector shadow
PASS G18.4 final rollback retained legacy REAL[] reader/data
no migrations changed
disposable_database=removed
```

The Golden proof covers exact error/path identifiers, English phrase recall,
Chinese lexical and bounded-bigram recall, semantic-only Dense ranking,
standalone context rewrite, cross-collection selection, and two unrelated
no-evidence cases. Both negative cases returned zero candidates. Repeated
original/rewrite outer RRF returned identical UUID order. After tombstoning the
first document, three BM25 and three vector shadow rows remained physically for
rollback, but both the hybrid shadow reader and legacy production reader
returned zero candidates for that collection; the separately selected current
collection still returned its one candidate.

The down sequence removed G18.4 first while proving G18.3 remained, then removed
G18.3 while retaining four legacy `REAL[]` rows and the production hybrid
function. All disposable containers and volumes were removed.

Known cutover debt is explicit: `pg_textsearch` may apply low-selectivity scope
filters after Top-K. The bounded 8x shadow overfetch is not yet a production
single-server budget. G18.5 must prove representative-corpus selectivity,
latency, RSS/CPU, concurrent indexing/deletion/reindex, restart, backup/restore,
and the reversible server-owned profile pointer before cutover.

Final quality gates passed: shell syntax, module completeness, placeholder and
secret scans, explicit role-grant review, `go vet ./...`, `go test ./...`, and
the seven-case evaluator (`precision=1`, false-citation rate `0`, no-evidence
accuracy `1`). The generic module/security scanners do not classify SQL files,
so the live role, SECURITY DEFINER, forged-insert, diagnostics-shape, and query
plan assertions remain the authoritative SQL security tests.

Next: commit G18.4 alone. G18.5 then promotes the reviewed PG17 DDL into the
normal migration path only after a verified restore to fresh PG17 storage.

## 2026-07-22 — G18.5A PG16-compatible retrieval profile pointer

The first cutover slice deliberately did not add `pg_textsearch`, pgvector
types, or a PostgreSQL 17 execution branch to the ordinary migrations. The
running independent service is still PostgreSQL 16, so migration `037` adds
only an extension-free server-owned pointer and a stable profiled reader. The
pointer starts at `legacy` revision `1`; the Go repository now calls the
profiled reader, which delegates row-for-row to the existing
`ts_rank + REAL[]` function.

`knowledge_set_retrieval_profile(expected, target, revision, reason)` uses a
row lock plus transaction advisory lock for compare-and-swap transitions. A
stale expected state raises `RAG_RETRIEVAL_PROFILE_CONFLICT`. Migration `037`
intentionally raises `RAG_RETRIEVAL_PROFILE_UNAVAILABLE` for
`pg17_bm25_pgvector_v1`, so an absent schema/backfill cannot be activated.
Successful future transitions append immutable history. Only
`rag_replay_operator` may mutate the pointer; `go_api_runtime` and
`rag_worker_executor` can execute only the reference-shaped candidate reader.

The down migration checks the active profile before dropping any object. A
non-legacy pointer raises
`RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY` inside the migration
transaction, retaining migration `037`, its tables, functions, and state.
Operators must first roll application traffic back to the legacy profile, then
remove the migration.

Disposable proof command:

```bash
./mm-chat/scripts/run-g18-profile-pointer-drill.sh
```

Final report: `/tmp/mm-chat-g18-profile-pointer.CdhPfb`.

```text
PASS PG16 migrations=37 authority=1 objects=2 generation=active projection=ready
PASS G18.5A profile=legacy parity=exact roles=bounded pg17=fail-closed
PASS G18.5A non-legacy rollback rejected atomically
PASS G18.5A rollback retained legacy reader
disposable_database=removed
```

The proof applied all migrations on synthetic PostgreSQL 16 data, compared the
complete legacy/profiled result rows, exercised the reader under both runtime
roles, and exercised idempotent/unavailable/conflicting pointer operations
under the replay role. It then forced a non-legacy head to validate atomic down
failure, switched back through the controlled function, rolled down/reapplied,
and confirmed a fresh restart state of `legacy` revision `1`. No production
database, bind-mounted data directory, provider, host port, or live retrieval
profile was touched; the exit trap removed every project-scoped container and
volume.

The Go repository call was also exercised against a separate disposable PG16
instance with `MM_CHAT_REQUIRE_POSTGRES_TESTS=true`:

```text
TestPostgresFetchQueryEvidenceCandidatesReturnsBoundedReferences  passed
profiled reader returned the expected bounded Dense reference     passed
temporary PG16 container                                          removed
```

After migration `037` changed the production manifest, the complete G18.4
shadow drill was rerun rather than relying on its earlier 36-migration report.
Report `/tmp/mm-chat-g18-hybrid-shadow.EC2Xg4` passed all seven BM25/Dense/RRF
cases, both unrelated zero-candidate cases, HNSW/BM25 index plans, deletion
invisibility, both rollback layers, and cleanup with migrations `1–37`.

Final G18.5A quality gates:

```text
gofmt / go vet ./...                         passed
go test -count=1 ./...                       passed
focused disposable-PG16 repository test     passed
recorded seven-case evaluator                passed
schema migration contract test               passed
bash syntax / diff whitespace                passed
SECURITY DEFINER paths / role grants          passed live
temporary G18 containers / volumes            0 / 0
```

The normal all-package Go run intentionally leaves
`MM_CHAT_TEST_DATABASE_URL` unset because older chat/browser-import fixtures
share one database and are not cross-package isolation-safe. Database behavior
for this slice is covered by the dedicated pointer drill and the focused
repository test, each with its own disposable PG16 database.

G18.5A does not complete Group 5. Effective production retrieval is still the
legacy reader, and Compose still targets PostgreSQL 16. G18.5B owns migration
`038`, the PG17 BM25/pgvector implementation, verified backfill, pointer
activation, operational/resource proofs, blue-green data-path cutover, and
rollback from the preserved PG16 backup.

Next: commit G18.5A alone. Then implement G18.5B on disposable PG17 storage;
do not alter the running database or `mm-chat/data/postgres` before backup and
restore gates pass.

## 2026-07-22 — G18.5B.1 disposable PG17 profile activation candidate

The first PG17 activation slice intentionally remained outside
`backend/migrations`. Adding a PG17-only migration while the independent stack
still runs PostgreSQL 16 would break an ordinary `migrate up`. The embedded
chain therefore remains `1–37`; the new `ops/g18-profile-cutover` module composes
the already-reviewed G18.3 pgvector and G18.4 BM25 DDL with the smallest profile
router replacement.

`knowledge_assert_pg17_retrieval_profile_ready()` resolves the active corpus
generation and its unique `mineru_jina_postgres_v1` Jina v4/1024 search profile.
It does not trust counts alone: every current source must join both projections
on immutable identity, hashes, visibility revisions, vector round-trip,
normalized exact terms, and derived BM25 text. Missing coverage raises
`RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE` before pointer history changes.

The PG17 setter acquires the vector, BM25, and pointer advisory locks in order
`3 -> 4 -> 5`, then performs readiness and compare-and-swap under the locked
pointer row. The profiled reader preserves the `UUID[], TEXT, REAL[], INTEGER`
input and reference-only result shape expected by Go. Under the PG17 profile it
validates the legacy query vector contract, casts to `VECTOR(1024)`, invokes the
reviewed hybrid implementation as `rag_projection_owner`, and returns only
immutable references plus fused RRF score. Per-lane diagnostics and source text
remain unavailable to production roles.

The first disposable run stopped at the fixture boundary and was discarded:
the G18.4 lexical fixture correctly requires its pgvector prerequisite to be
backfilled first. The harness was corrected to model the real partial state
(vector complete, BM25 absent), then prove that activation is still rejected.
No schema relaxation was made, and the failed disposable project was removed.

Final proof command:

```bash
./mm-chat/scripts/run-g18-profile-cutover-drill.sh
```

Final report: `/tmp/mm-chat-g18-profile-cutover.oTuY1M`.

```text
PASS PG17 migrations=37 authority=1 objects=2 generation=active projection=ready
PASS G18.5B.1 profile router candidate
PASS G18.5B.1 backfill=complete activation=pg17 roles=bounded negatives=0
PASS G18.5B.1 restart retained pg17 profile and reader
PASS G18.5B.1 controlled rollback restored exact legacy reader
PASS G18.5B.1 profile router candidate rollback
PASS G18.5B.1 rollback retained migration 037 and legacy reader
disposable_database=removed
```

The proof advanced `legacy@1 -> pg17_bm25_pgvector_v1@2`, returned the approved
identifier and semantic winners through `go_api_runtime` and
`rag_worker_executor`, and returned zero candidates for the frozen weather
negative. Runtime roles could not mutate the pointer or execute the private
diagnostic. PostgreSQL restart retained the pointer, readiness result, and
reader. Attempted router down while PG17 was active failed before mutation.
Controlled compare-and-swap rollback produced `legacy@3`, preserved both
transition records, restored complete row-level parity with the direct legacy
function, and then removed the router, BM25, and pgvector candidate layers.

Final G18.5B.1 quality gates:

```text
profile cutover live PG17 drill                passed
active-profile rollback guard                 passed atomically
bash syntax / diff whitespace                 passed
module README / DESIGN completeness           passed
go vet ./...                                   passed
go test -count=1 ./...                         passed
recorded seven-case evaluator                  passed
secret / placeholder scan                     passed
temporary cutover containers / volumes        0 / 0
```

The generic module/security scanners do not classify SQL as source code. The
live assertions over SECURITY DEFINER paths, role grants, private diagnostic
denial, reference-only return fields, activation state, and rollback behavior
remain the authoritative security proof for this operational SQL.

G18.5B.1 is not a production migration or cutover. It does not yet solve
projection maintenance during concurrent document publication or generation
rebuild, and it does not claim representative latency/RSS/CPU or real
backup/restore results. These are G18.5B.2 gates. Only after they pass may the
reviewed SQL become migration `038` on the restored PG17 target in G18.5B.3.

Next: commit G18.5B.1 alone, then build the concurrent publication/reindex and
resource qualification drill without touching the running PG16 service.

## 2026-07-22 — G18.5B.2a active-generation publication maintenance

The production embedding finalizer already publishes in the correct order:
materialization becomes `published`, the document/version authority becomes
current, and then `knowledge_document_projection_heads` is inserted or
advanced—all inside one transaction. G18.5B.2a uses that final head mutation as
the atomic PG17 maintenance boundary rather than adding a second worker queue
or letting the browser/API write accelerator tables.

When the pointer is legacy, the new head trigger is a no-op and activation
readiness remains responsible for closing any pre-activation gap. When the
pointer is PG17, the trigger calls the projection-owner materialization sync.
That function acquires advisory locks `3 -> 4`, validates that BM25 and vector
source views expose the same non-empty current materialization, inserts both
physical projections, and re-verifies every identity/content field before the
head mutation may commit. Any failure rolls back the entire surrounding
publication transaction.

The first fixture attempt correctly failed the existing document lifecycle
constraint because it tried to insert an `active` document before assigning a
current version. The fixture was corrected to follow the real transition
`processing -> active/current`; no constraint was weakened. The failed
transaction and disposable database were removed.

The extended proof command remained:

```bash
./mm-chat/scripts/run-g18-profile-cutover-drill.sh
```

Final report: `/tmp/mm-chat-g18-profile-cutover.hAggco`.

```text
PASS G18.5B.2a active projection maintenance
PASS G18.5B.2a concurrent publication fixture staged=2 heads=0
PASS G18.5B.2a concurrent publish=2 sync=atomic delete=hidden rows=retained
PASS G18.5B.1 restart retained pg17 profile and reader
PASS G18.5B.2a active projection maintenance rollback
PASS G18.5B.1 rollback retained migration 037 and legacy reader
disposable_database=removed
```

Two independent psql sessions inserted different document projection heads
concurrently while `pg17_bm25_pgvector_v1@2` was active. Both transactions
completed, both materializations appeared in vector and BM25 projections, and
readiness moved from four to six exact current rows. The production-shaped Go
reader returned the correct `LIVE_ALPHA` and `LIVE_BETA` children. Replaying one
materialization as `rag_replay_operator` inserted zero rows and re-verified one.

After tombstoning `LIVE_ALPHA`, it returned zero authorized candidates while
`LIVE_BETA` remained visible. All six vector and BM25 rows remained physically
available for rollback, and readiness correctly counted the five current
sources. PostgreSQL restart retained this state. After pointer rollback to
legacy, the maintenance trigger/function, router, BM25, and pgvector layers
rolled down in dependency order while migration `037` and legacy retrieval
remained intact.

G18.5B.2a closes active-generation publication drift only. A building/verified
generation is not yet the BM25 authority, so generation rebuild/reindex and
atomic corpus-head cutover still need a dedicated fence. Representative
latency, backfill time, index size, RSS/CPU, and backup/restore also remain for
G18.5B.2b.

Final G18.5B.2a quality gates:

```text
concurrent PG17 publication/delete drill       passed
maintenance + router active rollback guards   passed atomically
bash syntax / diff whitespace                 passed
go vet ./...                                   passed
go test -count=1 ./...                         passed
recorded seven-case evaluator                  passed
temporary cutover containers / volumes        0 / 0
```

Next: commit G18.5B.2a alone, then qualify generation cutover and resources on
a representative disposable corpus.

## 2026-07-22 — G18.5B.2b generation reindex and corpus-head fence

The active-only BM25 source was safe for reads but could not support a PG17
profile during a structure rebuild: publishing a document head in a `building`
generation invoked projection maintenance, yet the BM25 source hid that row and
forced the surrounding publication transaction to fail. Broadening the reader
source would have been worse because building and retired rows could enter
production retrieval before the corpus-head cutover.

The fix separates these responsibilities. The new
`knowledge_bm25_shadow_build_sources` admits current, published document heads
from `building`, `verified`, `active`, and `retired` generations, matching the
existing pgvector build source. BM25 insert validation, backfill, and
materialization sync use that build source. The original
`knowledge_bm25_shadow_sources` now wraps it with the active singleton corpus
head and remains the only BM25 reader authority.

`knowledge_assert_pg17_generation_ready(UUID)` verifies a target generation's
complete current-document coverage, one-to-one BM25/Dense source identity, and
exact physical vector/BM25 rows. A hardened BEFORE trigger on
`knowledge_corpus_projection_head.active_index_generation_id` acquires advisory
locks `3 -> 4` and runs this assertion whenever the PG17 retrieval profile is
active. Migration `032` promotion and migration `033` rollback both update the
target generation status before advancing that head, so both operations cross
the same final transactional fence without rewriting their existing APIs.

The synthetic reindex fixture created three current-document projections in a
building generation but published only one head first. That first head created
both physical rows while the active reader returned no candidate-generation
row. An attempted generation/head transition failed with
`RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE`; the caught exception subtransaction
left the old generation active and the candidate building. After publishing the
other two heads, exact readiness reported three documents and three paired
rows. Replaying both backfills inserted zero rows.

Final proof command:

```bash
./mm-chat/scripts/run-g18-profile-cutover-drill.sh
```

Final report: `/tmp/mm-chat-g18-profile-cutover.dK6RSH`.

```text
PASS G18.5B.2b generation cutover fence
PASS G18.5B.2b reindex fixture building_heads=1 expected_documents=3
PASS G18.5B.2b reindex partial=rejected promotion=3 rollback=exact
PASS G18.5B.1 restart retained pg17 profile and reader
PASS G18.5B.2b generation cutover fence rollback
PASS G18.5B.1 rollback retained migration 037 and legacy reader
disposable_database=removed
```

The successful switch served `LIVE_BETA` only from the candidate generation.
The reverse switch restored the exact prior child reference, while all three
candidate vector and BM25 rows remained immutable for diagnosis/rollback. The
profile stayed PG17 through restart, all candidate down scripts continued to
reject removal until the profile returned to legacy, and the embedded migration
manifest remained exactly `1–37`.

Final G18.5B.2b quality gates:

```text
building-generation + cutover PG17 drill       passed
incomplete cutover atomic rejection            passed
promotion / exact rollback reader proof        passed
restart / down guards / cleanup                 passed
bash syntax / diff whitespace                   passed
go vet ./...                                    passed
go test -count=1 ./...                          passed
recorded seven-case evaluator                   passed
temporary cutover containers / volumes         0 / 0
```

G18.5B.2b does not measure representative query latency, backfill duration,
index size, or PostgreSQL RSS/CPU, and it is not the real PG16 backup/PG17
restore. Those remain the isolated G18.5B.2c gate. The running PostgreSQL 16
service and `mm-chat/data/postgres` were not read, stopped, or modified.

Next: commit G18.5B.2b alone, then build the disposable representative-resource
and backup/restore qualification before freezing migration `038`.

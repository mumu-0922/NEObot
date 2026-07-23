# G18 BM25 and pgvector Retrieval Plan

Status: complete (2026-07-22). G18 promoted the independent server RAG path
from PostgreSQL `ts_rank` plus `REAL[]` cosine scans to true BM25 plus indexed
pgvector retrieval without weakening current authority, deletion, citation,
or rollback boundaries. The optional BGE-M3 comparison remains a separate
future experiment, not unfinished G18 scope.

## Execution rule

- Deliver one bounded group at a time.
- Test, record, and commit every completed group before starting the next.
- Keep the current production reader until shadow quality and operations gates
  pass.
- Keep source text, provider credentials, and private user data out of fixtures,
  reports, logs, and commits.
- Never mount the PostgreSQL 16 data directory into PostgreSQL 17.

## G18.1 Evaluation and relevance gate

Status: complete (2026-07-22).

- Add a versioned synthetic Golden Set covering exact identifiers, Chinese
  lexical/semantic questions, contextual rewrites, cross-collection ranking,
  and unrelated negatives.
- Add a deterministic evaluator for per-lane recall, final-context precision,
  negative false-citation rate, no-evidence accuracy, and P95 latency.
- Record the existing `ts_rank + REAL[] + jina-reranker-v3` baseline before
  changing storage.
- Calibrate against the complete Golden Set; do not infer that reranker scores
  are probabilities or raise a global threshold from isolated examples.
- Fail closed to ordinary Model/Web answering when configured reranking is not
  authorized, unavailable, or malformed; unreranked candidates cannot mint
  Knowledge citations.

Promotion baseline: all required lanes recalled their approved evidence,
final-context precision and no-evidence accuracy were `1.0`, negative
false-citation rate was `0`, and Knowledge-stage P95 was `25.402s` on the live
disposable synthetic set. The high P95 is owned by the contextual rewrite
provider path and remains a comparison guard for later storage groups.

## G18.2 PostgreSQL 17 image and restore drill

Status: complete (2026-07-22).

- Build and digest-pin a reviewed PostgreSQL 17 image containing
  `pg_textsearch` and pgvector.
- Restore a disposable PostgreSQL 16 backup into PostgreSQL 17 and verify
  migrations, authority rows, projections, generation heads, and objects.
- Prove rollback from the preserved PostgreSQL 16 backup.

Promotion proof: the isolated no-host-port harness built PostgreSQL `17.10`
with `pg_textsearch 1.3.1` and pgvector `0.8.5`, exercised real BM25 and vector
queries, rejected a PostgreSQL 16 data directory with exit `78`, and restored
the same synthetic logical backup into fresh PostgreSQL 17 and PostgreSQL 16
rollback databases. Both restores retained all `36` migrations, the authority
graph, object references, active generation head, published materialization,
and ready Parent/Child/Search projection. The source database was unchanged,
and the harness removed all disposable containers and volumes.

## G18.3 Shadow pgvector projection

Status: complete (2026-07-22).

- Add a generation/profile-bound `vector(1024)` projection beside `REAL[]`.
- Backfill compatible finite Jina v4 vectors transactionally without provider
  calls.
- Evaluate exact cosine first, then HNSW recall and latency; keep the existing
  reader in production.

Promotion proof: a PG17-only shadow schema copies only current, published,
ready Jina v4/1024 rows under an explicit generation and search profile. The
transactional backfill verifies count, full identity, visibility revisions,
float32 hash, vector round-trip, and norm; repeated execution is idempotent.
The seven frozen storage cases passed exact/HNSW parity, collection isolation,
unrelated no-evidence thresholds, immediate tombstone invisibility, zero/NaN
rejection, real HNSW plan use, and shadow-only rollback. No provider was called,
the production migration manifest remained `1–36`, and the current `REAL[]`
hybrid function remains the only production reader.

## G18.4 BM25 and hybrid dual read

Status: complete (2026-07-22).

- Add BM25 only for active, published, current-generation child chunks.
- Preserve identifier, path, error-code, phrase, Chinese, and semantic recall.
- Fuse BM25 and Dense lanes through deterministic RRF and emit only redacted
  shadow diagnostics.

Promotion proof: the PG17-only shadow admitted four synthetic child rows only
through the active corpus head and current published authority. A real
`pg_textsearch 1.3.1` index handled identifiers, paths, phrases, exact terms,
and bounded Chinese ideograph bigrams; the pgvector HNSW lane remained
generation/profile-bound. Both lanes fused through deterministic `k=60` RRF,
including the existing original/standalone-rewrite outer query lanes. Seven
frozen cases passed with both unrelated cases returning zero candidates. The
diagnostic surface returned references, hashes, ranks, and scores only. After
tombstoning, stale immutable shadow rows remained for rollback but immediately
disappeared from authority-bound reads. G18.4 and then G18.3 rolled back without
changing migrations `1–36` or the legacy `REAL[]` production reader.

## G18.5 Cutover and rollback

- Cut over behind a reversible server-owned retrieval-profile pointer.
- Prove restart, concurrent indexing, deletion, reindex, backup/restore,
  resource budget, and rollback.
- Retain `REAL[]` rollback data through the observation window.

### G18.5A PG16-compatible profile pointer

Status: complete (2026-07-22). Group 5 later completed under G18.5B.3b.

- Add migration `037` without PG17-only extension types so it can land safely
  on the then-running PostgreSQL 16 runtime.
- Route the Go candidate reader through a stable server-owned function while
  the pointer defaults to `legacy` revision `1`.
- Use an operator-only compare-and-swap transition function with immutable
  history, strict unavailable/conflict errors, and a non-legacy rollback guard.
- Prove exact legacy parity, bounded role grants, down/reapply, restart state,
  and disposable cleanup before adding any PG17 implementation.

Promotion proof: the disposable PG16 drill applied all `37` migrations, read
the synthetic candidate through both the legacy and profiled functions with
exact row parity, and proved that `go_api_runtime` and `rag_worker_executor`
cannot mutate profile state. The PG17 target failed closed, a forced non-legacy
down attempt failed atomically, and controlled return to legacy permitted
down/reapply while retaining the old reader. The Go production query now uses
the profiled function, but effective retrieval remains `ts_rank + REAL[]`.

### G18.5B PG17 implementation and controlled activation

Status: complete (2026-07-22).

- Promote the reviewed BM25/pgvector DDL into migration `038` after a fresh
  verified PG16 backup is restored into PostgreSQL 17 storage.
- Add verified backfill and the real `pg17_bm25_pgvector_v1` reader branch.
- Prove concurrency, deletion, reindex, restart, backup/restore, latency,
  PostgreSQL RSS/CPU, activation, and compare-and-swap rollback.
- Change Compose/data-path authority only after all disposable proofs pass;
  never start PG17 against the existing PG16 directory.

#### G18.5B.1 Disposable profile activation candidate

Status: complete (2026-07-22). G18.5B and Group 5 later completed under
G18.5B.3b.

- Keep the embedded migration chain at `1–37` while production is PG16.
- Connect the profile router to the reviewed PG17 shadow projections under an
  operational-only module.
- Reject activation until the active Jina v4/1024 generation has complete,
  identity/content-verified pgvector and BM25 coverage.
- Prove reference-only runtime reads, bounded privileges, restart persistence,
  active-profile rollback refusal, controlled legacy rollback, and removal of
  all PG17 candidate layers without harming migration `037`.

Promotion proof: a disposable PG17 database rejected activation with pgvector
ready but BM25 absent, then admitted exactly four verified rows in both
projections and advanced the pointer from `legacy@1` to
`pg17_bm25_pgvector_v1@2`. Go and worker roles returned the approved identifier
and semantic winners while a frozen unrelated query returned zero candidates.
The pointer and reader survived restart. Router down failed while PG17 was
active; compare-and-swap rollback produced `legacy@3`, restored exact legacy
row parity, and allowed all candidate objects to be removed while migrations
remained `1–37`.

#### G18.5B.2 Operations and resource qualification

Status: complete (2026-07-22). G18.5B later completed under G18.5B.3b.

- Add and prove a safe projection-maintenance path for documents published
  while the PG17 profile is active.
- Prove concurrent publish/delete, generation rebuild/reindex/cutover, restart,
  and backup/restore without a candidate-visibility gap.
- Measure representative BM25/pgvector latency, PostgreSQL RSS/CPU, index size,
  and backfill duration against the single-server budget.

##### G18.5B.2a Active-generation publication maintenance

Status: complete (2026-07-22). G18.5B.2 later completed under G18.5B.2c.

- Attach maintenance to the durable projection-head mutation already emitted
  by the embedding publication transaction.
- While PG17 is active, insert and fully verify both physical projections
  before the publication transaction can commit.
- Serialize sync and activation in one advisory-lock order; keep direct sync
  operator-only and idempotent.
- Prove two independent concurrent publications, query visibility, deletion
  invisibility with rollback-row retention, restart, and controlled rollback.

Proof: two prepared document heads were inserted from independent PostgreSQL
sessions after profile activation. Both acquired the projection critical
section, populated pgvector and BM25, passed exact readiness at six rows, and
returned their expected candidates through the production-shaped reader. A
manual replay inserted zero rows. Tombstoning one document immediately removed
it from authorized results while six immutable physical rows remained; exact
readiness adjusted to the five still-current sources. Restart and legacy
rollback remained green.

##### G18.5B.2b Generation reindex and corpus-head cutover

Status: complete (2026-07-22). G18.5B.2 later completed under G18.5B.2c.

- Bind the projection maintenance/backfill gate to generation rebuild and
  atomic corpus-head cutover so a new active generation never opens empty.
- Keep building-generation write admission separate from active-reader
  authority.
- Reject incomplete promotion without partial generation/head state, then
  prove complete promotion and exact rollback with immutable rows retained.

Proof: a three-document building generation published only one head first. Its
vector and BM25 rows were maintained, but it remained invisible to the active
reader and corpus-head cutover failed atomically. Publishing the remaining two
heads produced complete paired projections; idempotent backfill inserted zero
rows. Promotion returned only new-generation references, then rollback restored
the exact old references while retaining all candidate projection rows.

##### G18.5B.2c Resource and restore qualification

Status: complete (2026-07-22). G18.5B.3 later completed under G18.5B.3b.

- Run representative corpus latency, backfill duration, index-size, RSS/CPU,
  restart, backup/restore, and legacy rollback measurements.

Proof: under a hard `1 GiB / 2 CPU` PG17 container, one atomic document-head
publication populated and verified 4096 vector and BM25 rows in `11.019s`.
Thirty production-shaped hybrid queries returned every intended child with
`230.241ms` P95 and `241.324ms` maximum latency. The physical vector/BM25
tables plus indexes used `64,446,464` bytes, and cgroup memory peaked at
`347,545,600` bytes, below the `900 MiB` gate.

The first resource run exposed a corpus-wide authority rejoin rather than a
BM25 or pgvector accelerator failure. Candidate-driven `LATERAL` authority
resolution reduced P95 from more than two seconds to about 230ms while
preserving current-generation, current-document, selected-collection, and
reference-only boundaries. The active PG17 database then survived restart,
custom-format logical backup, SHA-256 verification, restore into a fresh
database, migration idempotence, exact active/physical row counts, runtime
reader behavior, role grants, controlled legacy rollback, and complete
disposable cleanup.

#### G18.5B.3 Formal migration and blue-green Compose cutover

Status: complete (2026-07-22).

- Freeze the reviewed candidate as migration `038` only after a verified live
  PG16 backup is restored into fresh PG17 storage.
- Apply, backfill, activate, switch Compose/data-path authority, and retain the
  PG16 backup plus legacy `REAL[]` data through the observation window.

##### G18.5B.3a Restored-live-data migration qualification

Status: complete (2026-07-22).

- Create migration `038` from the reviewed pgvector, BM25, profile-router,
  active-publication, and generation-fence modules, with exact PG17/preload/
  extension-version gates and no `psql` meta-commands.
- Restore the owned PG16 logical backup and sanitized PG17-compatible role
  dump into an isolated fresh PG17 volume before applying the migration.
- Prove that the embedded migration runner creates both extensions itself,
  applies `036 -> 037 -> 038`, backfills current live authority, activates the
  profile, enforces role boundaries, survives restart, rejects active rollback,
  and supports controlled down/re-up replay.

Proof: the checksummed live backup restored with `36/36` migrations, two
collections, four documents, and 13 ready search rows. After removing the two
manually installed but unused extensions, the current migration binary applied
`037` and `038`; migration `038` recreated pgvector `0.8.5` and
`pg_textsearch 1.3.1` in its own runner transaction. Eleven current-authority
Jina v4/1024 rows backfilled and verified in both projections. Both runtime
roles returned reference-only candidates while retaining no direct projection
or private-diagnostic access. Active-profile down failed atomically; switching
to legacy allowed down, retained all 24 legacy `REAL[]` rows and both
extensions, and re-up/backfill/activation replay completed at profile revision
`4`. Restart retained migration head `038`, the active profile, and exact
`11/11/11` readiness.

##### G18.5B.3b Production Compose/data-path authority cutover

Status: complete (2026-07-22).

- Re-enter a short write-stop window and take a fresh final PostgreSQL/MinIO
  backup immediately before the authority switch.
- Change the production Compose database to the pinned PG17 image and a fresh
  PG17 data path; never reuse `mm-chat/data/postgres` as the PG17 directory.
- Restore, migrate, backfill, activate, start application traffic, and run
  post-cutover health/chat/Knowledge checks with the PG16 data directory and
  backup retained as rollback anchors.

Proof: application writers stopped before the final owner-preserving and
portable PostgreSQL dumps, role dump, MinIO archive, local configuration, and
provider-keyring rollback artifacts were created and checksum-verified under
owner-only permissions. The final source snapshot was PostgreSQL `16.13` at
migration `036`, with two active collections, four active documents, 13 ready
of 24 Search rows, and 124 conversations.

The backup restored into fresh `data/postgres17` under the pinned PostgreSQL
`17.10`, pgvector `0.8.5`, and pg_textsearch `1.3.1` image. Migration
`037 -> 038`, `11/11` current-authority dual-projection backfill, and
`pg17_bm25_pgvector_v1@2` activation completed before Compose traffic reopened.
The Compose-owned database then passed migration no-op, exact live-count,
reference-only runtime-reader, privilege, 41/41 active MinIO object, direct and
proxied HTTP health, PostgreSQL restart, and application reconnect checks.

The inherited local Go `DATABASE_URL` still used the database owner. Cutover
closed that configuration drift by rotating `neo_chat_api`, updating only the
private env file, and proving the live backend connection now uses the
dedicated `go_api_runtime` member without direct projection access. The old
physical `data/postgres` PG16 directory, final backups, MinIO archive, and
legacy `REAL[]` rows remain intact for the observation/rollback window.

## G18.6 Optional BGE-M3 shadow benchmark

- Use a separate immutable embedding generation and never mix vector spaces.
- Compare BGE-M3 and Jina v4 on the same frozen Golden Set.
- Require an explicit decision and full rebuild/cutover plan before any
  production model switch.

## Verification

- Focused Go evaluator/chat tests for G18.1.
- Disposable database integration tests for every storage group.
- Backend `gofmt`, `go vet ./...`, and `go test ./...` after each Go group.
- Python checks when the RAG worker is touched.
- Live provider/database proof only with synthetic fixtures and complete
  cleanup.

## Rollback

G18.1 adds no schema. G18.2 is blue-green and retains the PostgreSQL 16
backup. G18.3 and G18.4 are shadow-only until G18.5. The production reader and
legacy `REAL[]` projection remain the rollback anchors through observation.

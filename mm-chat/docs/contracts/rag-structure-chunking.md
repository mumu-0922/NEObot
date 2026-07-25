# RAG Structure-Aware Chunk Planning Contract

## 1. Scope / Trigger

This contract covers G11.9D.1 deterministic planning, G11.9D.2.1 Native
projection, G11.9D.2.2 admitted MinerU page-element projection,
G11.9D.2.3a candidate-generation allocation, and G11.9D.2.3b leased candidate
parse projection, G11.9D.2.3c real passage-embedding completeness,
G11.9D.3a generation verification, G11.9D.3b deletion/failure fencing, and
G11.9D.3c atomic cutover/rollback. Migration `043` adds bounded Child hydration
and adaptive Parent expansion. Migration `044` supersedes the old runtime
cutover grant: only `rag_replay_operator` may begin, verify, activate, or roll
back a Structure Profile v2 candidate. Migration `045` makes the exact matched
Child Search projection the Citation locator authority. Migration `046`
requires an exact, confirmed, and immutably audited operator action before a
verified Candidate can be abandoned.

Migration `050` permanently retires Jina execution and supersedes every older
Jina handler, Capture, evaluation, activation, and rollback statement in this
document. Sections that record G11.9D pre-050 live proofs are historical evidence
only. Until the BGE Candidate is explicitly activated, the historical Active
Generation may serve only its same-Generation BM25/Citation projection.

## 2. Signatures

```python
plan_structure_chunks(
    units: tuple[StructuredTextUnit, ...],
) -> StructureChunkPlan
```

```python
StructuredTextUnit(
    ordinal: int,
    kind: str,
    text: str,
    heading_path: tuple[str, ...] = (),
)
```

The output contains ordered `ParentChunkPlan` and `ChildChunkPlan` records. Each
fragment references only `unit_ordinal` plus a zero-based half-open UTF-8 byte
range. D.2 must map those references back to validated Canonical IR blocks and
clip their existing locators; it must not invent source coordinates.

## 3. Contracts

- Input units are non-empty, contiguous in reading order, and use the closed
  heading/paragraph/list/table/code/formula/native structural kind set.
- Planning is pure and deterministic: no network tokenizer download, database,
  clock, randomness, or global mutable state. The packaged tokenizer artifact
  is loaded locally and verified by SHA-256 before planning.
- Token authority is the frozen `cl100k_base` ordinary-text encoding. Parent,
  Child, overlap, and derived-context limits count the exact final rendered
  text, including admitted joiners and prefixes; UTF-8 byte ranges remain the
  source-location authority, not the size estimate.
- Typical Children target 300–500 tokens with a 400-token center and hard cap 650. Adjacent children reuse exact atom ranges for up to 100 tokens, targeting
  about 64.
- Parents target 1,200–1,600 tokens with a hard cap of 2,000. A Parent never
  crosses a `heading_path` change.
- Table rows, code, formula, heading, and other protected units remain atomic
  while they fit. Oversized structures route through type-specific boundaries
  first: table row groups, code logical regions, JSON subtree paths, slide
  shapes, or formula-safe boundaries. A tokenizer-aligned hard cut is the final
  bounded fallback and always remains UTF-8 safe.
- The planner caps one unit at 4 MiB, the document at 32 MiB, and unit count at
  100,000. Runtime admission may be stricter.
- Output references preserve source ordering and Parent containment. Overlap
  ranges must be byte-identical ranges present in the immediately preceding
  sibling Child.

## 4. Validation & Error Matrix

| Condition                                     | Result                   |
| --------------------------------------------- | ------------------------ |
| Empty/non-tuple or more than 100,000 units    | `StructureChunkingError` |
| Non-contiguous unit ordinal                   | `StructureChunkingError` |
| Unknown structural kind                       | `StructureChunkingError` |
| Empty, NUL, invalid scalar, or oversized text | `StructureChunkingError` |
| Invalid/oversized heading path                | `StructureChunkingError` |
| No UTF-8-safe forward boundary                | `StructureChunkingError` |
| Parent or Child exceeds its hard token cap    | `StructureChunkingError` |

Errors are fixed descriptions and must not include source text.

## 5. Good / Base / Bad Cases

- Good: a long multilingual section yields multiple Parents and 300–500-token
  Children, with exact adjacent overlap and no split UTF-8 scalar.
- Base: a short heading, paragraph, or table row stays below target size and
  remains one lossless source range.
- Bad: a malformed ordinal, unknown node kind, empty source unit, or unbounded
  document fails before any projection/provider side effect.

## 6. Tests Required

- Unit: deterministic replay, section-bound Parents, target/hard token bounds,
  exact adjacent overlap, protected table-row atomicity, multilingual UTF-8
  slicing, and every invalid-input class.
- D.2 integration: Native and MinerU Canonical IR/Chunk Manifest projection must
  preserve headings, paragraphs, lists, table rows, hashes, and clipped source
  locators while satisfying existing lineage/hash validators.
- D.3 live: build a new generation from active documents without re-upload,
  embed and verify all planned Children, keep old generation queryable on
  failure/deletion, then atomically cut over and prove citation anchors.

## 7. Wrong vs Correct

Wrong: split arbitrary bytes, flatten every structural node into one paragraph,
reuse source text without marking exact adjacent overlap, or mutate the active
generation while planning.

Correct: plan only validated source ranges, preserve heading boundaries and
protected units, keep the function pure, and leave persistence/provider/cutover
to separately verified D.2/D.3 stages.

## 8. G11.9D.2.1 Native Structural Artifact Projection

`build_native_structure_artifacts(...)` is the deterministic mapper from one
source-bound `NativeDocument` to projection-ready Canonical IR v2 and Chunk
Manifest v2. It is deliberately not called by `NativeSandboxParserGateway`
until the new chunk profile has a separately staged Search Profile and Index
Generation.

- headings and standalone text nodes become individual blocks;
- list items aggregate their descendant text once, while table rows aggregate
  cells with a fixed `" | "` separator and do not duplicate nested paragraphs;
- external-target fragments are excluded from indexed text;
- the Canonical text buffer separates blocks with one newline, while Chunk
  Manifest v2 uses its frozen `adjacent` empty joiner or `block_separator`
  double-newline joiner;
- heading paths contain logical heading block IDs, never display titles;
- every source unit, structure owner, block, provenance record, span, Parent,
  Child, profile, and artifact-set identity is deterministic;
- identity fragments are clipped using exact UTF-8/scalar/line positions;
  syntax-decoded fragments retain their verified coarse source range instead
  of inventing byte coordinates;
- Child overlap reuses the byte-identical previous-child fragment and records
  `previousChildOrdinal`, `overlapGroupId`, and bounded overlap tokens;
- Parent `sectionOwnerSeedId` is the structure owner of its first block and
  Child ordinals remain globally contiguous for Postgres projection.

Required proof includes DOCX heading/paragraph/table-row mapping, Markdown
heading/list/table/code mapping, long multilingual UTF-8 splitting and exact
overlap, offline JSON Schema validation, and
`build_postgres_projection_batch(...)` row construction. Runtime gateway
replacement, Postgres staging, real Jina spend, and generation cutover are not
evidence for this slice and must not occur here.

## 9. G11.9D.2.2 MinerU Structural Artifact Projection

`build_mineru_structure_artifacts(...)` consumes only the already decoded,
hash-bound `MinerULocalBatchCanonicalMappingInput`. Its current admitted
structure contract accepts the frozen synthetic `pages[].elements[]` shape and
the live-provider `pdf_info[]` shape, both with contiguous zero-based page
indexes, positive page geometry, bounded page BBoxes, and closed text-bearing
kinds.

- heading/title, text/paragraph, list/list-item, quote, code, table,
  formula/equation, footnote/`ref_text`, header, footer, and `page_number` map
  to Canonical text blocks;
- provider-classified page numbers are preserved as non-indexable footer
  evidence; an explicit `text` block with `lines=[]` is an empty provider
  placeholder and is skipped, while malformed or unknown text-bearing blocks
  still fail closed;
- tables render deterministically with `" | "` between cells and newline
  between rows;
- non-text image elements are ignored; an unknown element carrying text fails
  closed so content cannot disappear silently;
- heading ancestry uses logical block IDs and planner section paths;
- page objects and block/chunk locators retain admitted page/BBox geometry;
  clipping narrows only the canonical byte anchor;
- identities bind source, archive, and role digests under a MinerU-specific
  mapper identity while both Native and MinerU manifests bind the one shared
  `STRUCTURE_CHUNK_PROFILE_HASH` required by a mixed-format Index Generation;
- compatibility `full.md` remains admitted but is not structure authority.
- live `pdf_info[]` pages read `para_blocks` plus `discarded_blocks`, order
  blocks by BBox/index, join `lines[].spans[].content`, and convert PDF point
  geometry to integer milli-points; unknown text-bearing blocks still fail
  closed.
- live nested tables read only the observed
  `table.blocks[].table_body.lines[].spans[].html` lane. A bounded
  `HTMLParser` extracts `th`/`td` character data, decodes character references,
  escapes source pipes, and emits deterministic rows. The mapper never
  executes or retains provider HTML, and malformed, nested, or empty cell/row
  state fails closed.

Tests cover the frozen synthetic MinerU heading/text/table/formula corpus,
page-BBox projection, deterministic replay, schemas, Postgres DTO projection,
long multilingual UTF-8/overlap behavior, and the observed real-provider
`pdf_info[]` archive shape, including a caption plus nested Table HTML.
Unsupported provider shape must fail closed.

## 10. G11.9D.2.3a Candidate Generation Rebuild Allocator

`knowledge_begin_structure_generation_rebuild(...)` is the sole mutation
boundary for beginning a structure rebuild. The `SECURITY DEFINER` function is
owned by `rag_projection_owner` and executable by `go_api_runtime`.

### Signature

```sql
knowledge_begin_structure_generation_rebuild(
  index_profile_id UUID,
  search_profile_id UUID,
  generation_id UUID,
  chunk_profile_hash TEXT,
  base_profile_hash TEXT,
  parser_manifest_hash TEXT,
  search_profile_hash TEXT,
  build_snapshot_hash TEXT,
  allocations JSONB
) RETURNS TABLE(
  candidate_generation_id UUID,
  allocated_document_count BIGINT,
  active_generation_id UUID
)
```

Each allocation object contains lower-case UUID strings `documentId`,
`materializationId`, and `jobId`, plus a 64-character lower-case SHA-256
`requestHash`.

### Contracts

- it locks the corpus projection head and requires an existing active
  generation;
- it rejects any existing `building` or `verified` candidate;
- the supplied allocation set must contain every and only current active,
  available document exactly once;
- it clones active provider configuration into caller-identified shared
  Index/Search Profiles bound to the shared structure chunk profile;
- it creates one `building` generation and projection-state row, then one
  `staging` materialization and pending `parse/reprocess` job per document;
- job processing authority is inherited from that document's latest admitted
  parse job; IDs and request hashes are supplied by the trusted Go caller;
- it never calls `knowledge_promote_index_generation` and never updates
  `active_index_generation_id`.

### Validation & Error Matrix

| Condition                                          | Error                                               |
| -------------------------------------------------- | --------------------------------------------------- |
| Null ID, invalid hash, non-array/empty allocations | `RAG_STRUCTURE_REBUILD_ARGUMENT_INVALID`            |
| No active generation                               | `RAG_STRUCTURE_REBUILD_ACTIVE_GENERATION_MISSING`   |
| Existing building/verified candidate               | `RAG_STRUCTURE_REBUILD_CANDIDATE_EXISTS`            |
| Missing active Search Profile                      | `RAG_STRUCTURE_REBUILD_ACTIVE_PROFILE_MISSING`      |
| Count, uniqueness, or exact document set mismatch  | `RAG_STRUCTURE_REBUILD_ALLOCATION_COVERAGE_INVALID` |
| Invalid allocation UUID/hash shape                 | `RAG_STRUCTURE_REBUILD_ALLOCATION_INVALID`          |
| Document or latest parse authority unavailable     | `RAG_STRUCTURE_REBUILD_DOCUMENT_INVALID`            |

### Good / Base / Bad Cases

- Good: all active documents are allocated once and receive staging rows while
  the returned active generation ID remains unchanged.
- Base: a single active document produces one materialization and one pending
  parse job in the candidate.
- Bad: a same-size list substitutes another UUID, omits a document, duplicates
  a document, or races an existing candidate; the whole call rolls back.

### Tests Required

The migration proof must run against a disposable clone, cover missing and
same-cardinality wrong allocation sets, reject a second candidate, verify the
active generation is unchanged, and delete the clone. Real MinerU/Jina work,
candidate projection, verification, and cutover remain later D.2.3/D.3 slices.

### Wrong vs Correct

Wrong: compare only JSON array count and distinct count, which permits a
same-cardinality substituted document set, or create candidate rows before
locking the corpus head.

Correct: compare exact active-document membership under the head lock, let the
function transaction roll back every partial write, and leave promotion to a
separate verified boundary.

## 11. G11.9D.2.3b Leased Candidate Parse Projection

`knowledge_resolve_parse_chunk_profile(...)` is the fenced read boundary used
after a parse job is leased and before parser selection.

```sql
knowledge_resolve_parse_chunk_profile(
  job_id UUID,
  worker_id UUID,
  lease_token UUID,
  index_generation_id UUID,
  materialization_id UUID
) RETURNS TABLE(chunk_profile_hash TEXT)
```

### Contracts

- the supplied job must still be `processing`, owned by the supplied worker and
  lease token, unexpired, and bound to the supplied generation/materialization;
- the materialization must be `staging` and the generation must be
  `building`, `verified`, or `active`;
- the returned value comes from the generation's bound Index Profile, never
  from request data or parser authority;
- the baseline profile routes to the existing Native/MinerU text parser, while
  the shared structure profile routes to `NativeStructureSandboxParserGateway`
  or `MinerUStructureArchiveParserGateway` according to admitted processor
  authority;
- an unknown profile or processor fails closed before projection;
- successful parse projection may create one pending `passage_embedding` job,
  but a parse-only worker must not claim it.

Migration `030_rag_processing_job_replay_timestamp_fix` also freezes one
`replayed_at` value for replay `available_at`, `created_at`, `updated_at`, and
the replay audit row. This preserves `available_at >= created_at` even when the
database clock advances between expressions.

### Validation & Error Matrix

| Condition                                                                                                                          | Result                                     |
| ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| Null ID or zero lease token                                                                                                        | `RAG_PARSE_CHUNK_PROFILE_ARGUMENT_INVALID` |
| Wrong/expired lease, non-parse job, legacy-unbound job, mismatched binding, non-staging materialization, or unavailable generation | `RAG_PARSE_CHUNK_PROFILE_MISSING`          |
| Bound hash is neither baseline nor shared structure                                                                                | `NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED`  |
| Processor is neither admitted MinerU nor Native                                                                                    | `NATIVE_PARSER_AUTHORITY_UNSUPPORTED`      |
| Real MinerU layout loses/changes text-bearing shape                                                                                | `MINERU_STRUCTURE_ARTIFACT_INVALID`        |

### Good / Base / Bad Cases

- Good: a leased building-generation PDF resolves the shared hash, maps the
  real archive to page-BBox blocks, and leaves its embedding successor pending.
- Base: an active-generation document resolving the baseline hash continues
  through the existing parser with no artifact-profile behavior change.
- Bad: an expired lease, substituted materialization, unknown profile, or
  unknown text-bearing provider block fails before projection.

### Tests Required

- Migration tests must cover the function signature, owner/grant, lease/status/
  materialization/generation fences, and down migration.
- Unit tests must cover baseline and shared routing for both authorities,
  unsupported identities, Postgres row validation, real `pdf_info[]` mapping,
  signed-URL redaction, and replay timestamp SQL shape.
- Disposable-clone integration must assert the exact live-proof boundary below
  and must clean up even on failure.

### Wrong vs Correct

Wrong: choose the structure parser from request MIME, processor, or a caller
hash alone, which could project one generation under another profile.

Correct: resolve the profile through the current leased job and its staging
materialization, then combine that server-owned profile with admitted processor
authority before selecting a parser.

### Live Proof Boundary

A disposable clone must stage the complete active corpus and process exactly
one real MinerU PDF plus the Native documents. Required evidence is:

- every document's latest candidate parse succeeds;
- all candidate materializations remain `staging` and all Child chunks bind
  the shared structure profile;
- PDF blocks retain `page_bbox` locators;
- passage-embedding jobs exist only as `pending` and no Jina call is consumed;
- the old active generation ID is unchanged;
- signed result URLs never appear with query parameters in logs;
- the clone, temporary containers, provider-result proxy, and downloaded
  archive are removed after proof.

This slice does not prove passage embeddings, candidate completeness,
verification, cutover, or live citations. Those remain later D.2.3/D.3 work.

## 12. G11.9D.2.3c Candidate Passage Embedding Completeness

### Scope / Trigger

After every candidate parse projection succeeds and enqueues exactly one
passage-embedding job, the existing Jina/Postgres handler may embed and publish
each candidate materialization. This slice proves that path on the shared
structure profile; it does not verify or activate the generation.

### Signatures

```sql
knowledge_fetch_passage_embedding_candidates(
  job_id UUID, worker_id UUID, lease_token UUID, materialization_id UUID
)

knowledge_stage_passage_embedding(
  job_id UUID, worker_id UUID, lease_token UUID, materialization_id UUID,
  child_chunk_id UUID, embedding_vector REAL[], embedding_vector_sha256 TEXT
)

knowledge_assert_materialization_search_complete(
  materialization_id UUID, expected_child_count BIGINT,
  expected_embedding_model_id TEXT, expected_embedding_dimensions INTEGER
)

knowledge_complete_embedding_and_publish(
  embedding_job_id UUID, worker_id UUID, lease_token UUID,
  materialization_id UUID
)
```

Provider input is ordered Child `lexical_text`; provider output must use
`jina-embeddings-v4`, task `retrieval.passage`, and exactly 1024 finite float32
values per Child.

### Contracts

- fetch, stage, and completion remain fenced by the current processing job,
  worker, nonzero lease token, lease expiry, generation, and materialization;
- every provider vector must correspond one-to-one and in order with the
  fetched Child IDs; count, ID, dimension, and finite-value drift fails closed;
- staged vector hashes cover the exact network-to-Postgres float32 lane bytes;
- materialization completeness requires every Child search projection to be
  `ready`, 1024-dimensional, model-bound, lineage-equal, and vector-present;
- completion publishes only that candidate materialization and its
  generation-scoped document projection head, then commits the embedding job;
- completion does not set the Index Generation to `verified`, update the active
  generation ID, or call `knowledge_promote_index_generation(...)`.

### Validation & Error Matrix

| Condition                                                     | Result                                                 |
| ------------------------------------------------------------- | ------------------------------------------------------ |
| Missing/stale lease or substituted materialization            | `RAG_STALE_JOB_LEASE` or materialization-missing error |
| Empty/duplicate/malformed candidate rows                      | fixed embedding-candidate error                        |
| Provider count, Child ID, dimension, or finite-value mismatch | fixed Jina/handler vector error                        |
| Stage target does not match immutable Child/search lineage    | `RAG_PASSAGE_EMBEDDING_TARGET_MISSING`                 |
| Ready search count differs from Child count                   | `RAG_SEARCH_PROJECTION_INCOMPLETE`                     |
| Source authority changed before publish                       | `RAG_EMBEDDING_COMPLETION_AUTHORITY_STALE`             |

### Good / Base / Bad Cases

- Good: all mixed-format candidate Children receive real 1024-dimensional
  vectors, all materializations publish, and the candidate exactly covers the
  active document set while the active generation remains unchanged.
- Base: one Child produces one ready search projection and one published
  materialization through a single embedding job.
- Bad: one missing vector, stale lease, changed ACL/visibility/processing
  revision, or unmatched Child lineage rolls back completion.

### Tests Required

- Unit/integration tests cover Jina request task/model, response admission,
  float32 hash stability, candidate/vector correspondence, fenced Postgres
  calls, completeness rejection, and terminal-commit behavior.
- Disposable-clone live proof uses no mocks, asserts exact document coverage,
  successful parse and embedding jobs, published manifest/result hashes, ready
  vectors, candidate projection heads, and unchanged active generation.
- Logs and temporary credential-bearing files must be scanned/removed after
  proof.

### Wrong vs Correct

Wrong: mark a generation ready because all embedding jobs are terminal, without
checking exact document coverage, Child/search lineage, vectors, and published
materialization hashes.

Correct: prove every materialization independently through the fenced handler,
then leave generation-wide manifest/count verification and atomic cutover to
D.3.

### Live Proof Boundary

The three-document disposable clone completed three parse and three real Jina
passage-embedding jobs on their first attempts. All three candidate
materializations published with manifest/result hashes; all three Children had
ready shared-profile 1024-dimensional vectors; all three generation-scoped
document projection heads pointed at published materializations. Exact active
document coverage passed while the candidate remained `building` and the old
generation remained active.

The candidate projection-state Parent/Child counters, generation manifest,
`verified` transition, deletion-fence proof, promotion, and live citations are
deliberately deferred to G11.9D.3.

## 13. G11.9D.3a Generation Completeness Verifier

### Scope / Trigger

After D.2.3 publishes every candidate materialization, one generation-wide
transaction proves the candidate is complete and freezes its manifest. This is
the only allowed `building -> verified` boundary and cannot activate it.

### Signature

```sql
knowledge_verify_structure_generation(
  index_generation_id UUID,
  expected_head_revision BIGINT,
  expected_chunk_profile_hash TEXT
) RETURNS TABLE(
  candidate_generation_id UUID,
  artifact_manifest_hash TEXT,
  document_count BIGINT,
  block_count BIGINT,
  parent_count BIGINT,
  child_count BIGINT
)
```

### Contracts

- the corpus head is locked and must still have the expected revision and a
  different active generation;
- the candidate must be `building` with `building` projection state, or an
  already `verified`/`ready` candidate replaying the same manifest;
- candidate published document/version/file/content tuples must exactly equal
  the current active/available corpus set;
- the latest Parse and Passage Embedding job for every materialization must be
  `succeeded`, and every candidate document head must point to its published
  materialization;
- parser artifact sets must bind the candidate Index Profile and contain
  blocks; first verification marks those sets `verified` in the same
  transaction;
- every materialization has at least one Parent and Child, every Parent has a
  Child, and every Child has an immutable-lineage-equal ready search row under
  the expected shared profile and Jina 1024 model;
- Parent/search locator summaries must match and contain an admitted primary
  locator;
- the deterministic manifest hashes ordered materialization, parser-artifact,
  Block locator/content, Parent locator/content, Child content/lineage, and
  embedding-vector-hash row digests plus generation/profile/build/count inputs;
- success atomically writes the same manifest to generation and projection
  state, freezes document/Parent/Child counts, changes generation to `verified`
  and state to `ready`, but never updates `active_index_generation_id`.

### Validation & Error Matrix

| Condition                                                        | Result                                       |
| ---------------------------------------------------------------- | -------------------------------------------- |
| Invalid ID/head/hash argument                                    | `RAG_STRUCTURE_VERIFY_ARGUMENT_INVALID`      |
| Head revision stale, missing active head, or candidate is active | `RAG_STRUCTURE_VERIFY_HEAD_STALE`            |
| Candidate/status missing                                         | `RAG_STRUCTURE_VERIFY_CANDIDATE_MISSING`     |
| State/readiness/outbox floor inconsistent                        | `RAG_STRUCTURE_VERIFY_STATE_INVALID`         |
| Index/Search/Jina/shared profile mismatch                        | `RAG_STRUCTURE_VERIFY_PROFILE_MISMATCH`      |
| Exact current document tuple coverage differs                    | `RAG_STRUCTURE_VERIFY_COVERAGE_INVALID`      |
| Latest Parse/Embedding job incomplete                            | `RAG_STRUCTURE_VERIFY_JOBS_INCOMPLETE`       |
| Published document heads incomplete                              | `RAG_STRUCTURE_VERIFY_HEADS_INCOMPLETE`      |
| Parser artifacts/Blocks incomplete                               | `RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE`  |
| Parent/Child/vector/locator lineage incomplete                   | `RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE` |
| Verified replay recomputes different manifest/counts             | `RAG_STRUCTURE_VERIFY_REPLAY_MISMATCH`       |

### Good / Base / Bad Cases

- Good: the complete mixed-format candidate freezes one deterministic manifest,
  transitions to `verified/ready`, and replays to the identical hash/counts.
- Base: one published document with one Block, Parent, Child, ready vector, and
  successful job pair verifies without changing the active head.
- Bad: missing vector, stale head revision, substituted document/version,
  unknown profile, unmatched locator, or incomplete latest job aborts the whole
  transaction.

### Tests Required

- Migration tests assert signature, ownership/grant, closed errors, manifest
  domain, state transitions, rollback, and absence of promotion/active-head SQL.
- Disposable-clone proof must run a real D.2.3 candidate, call verification
  twice to prove deterministic replay, and show generation/state manifest
  equality plus frozen counts.
- A transactional negative test removes one ready vector and must receive
  `RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE`; rollback must restore the
  verified candidate and all vectors.
- Formal migration/head state and temporary-resource cleanup must be proved.

### Wrong vs Correct

Wrong: trust terminal job counts or caller-supplied counts and mark the
candidate verified without hashing immutable projection evidence.

Correct: derive exact coverage/counts and ordered row digests inside the locked
database transaction, persist one bound manifest, and leave activation to the
separate D.3c promotion gate.

### Live Proof Boundary

The disposable three-document candidate verified as 3 documents, 10 Blocks,
3 Parents, and 3 Children. Immediate replay returned the identical manifest and
counts. Removing one ready vector caused the closed projection-incomplete error;
the aborted transaction restored all three vectors and the verified/ready
state. The old generation remained active at the same head revision.

Atomic promotion and live citations remain G11.9D.3c.

## 14. G11.9D.3b Deletion/Promotion Fence and Candidate Failure

### Scope / Trigger

After D.3a verifies a candidate, promotion must revalidate current corpus
membership under the same corpus-head lock used by document deletion. If a
delete makes that candidate stale, the candidate must fail without changing
the active generation and a replacement rebuild must remain possible.

### Signatures

```sql
knowledge_promote_index_generation(
  index_generation_id UUID,
  expected_head_revision BIGINT,
  manifest_hash TEXT
) RETURNS BOOLEAN

knowledge_fail_structure_generation(
  index_generation_id UUID,
  expected_head_revision BIGINT,
  expected_manifest_hash TEXT,
  failure_code TEXT
) RETURNS BOOLEAN
```

### Contracts

- promotion locks the expected corpus head before reading candidate state;
- promotion accepts only the expected `verified/ready` candidate and manifest,
  resolves its persisted chunk-profile hash, and calls
  `knowledge_verify_structure_generation(...)` inside the same transaction;
- the verifier must recompute the same manifest against the current corpus
  before any active-generation mutation is allowed;
- the existing document-delete transaction locks the same corpus-head row
  before tombstoning its document/version, so delete and promotion serialize;
- fail rollback locks the expected head and candidate rows, requires the exact
  manifest, and atomically changes only `verified -> failed` and
  `ready -> failed` while preserving the active generation;
- replaying the same failed candidate, manifest, head, and failure code returns
  success without another mutation; a conflicting replay fails closed;
- failed generations no longer match the one-`building|verified`-candidate
  unique index, so a replacement rebuild may be allocated immediately;
- migration 032 grants fail rollback to `go_api_runtime`, but explicitly
  revokes successful promotion from that role. D.3c owns the first successful
  cutover.

### Validation & Error Matrix

| Condition                                          | Result                                                            |
| -------------------------------------------------- | ----------------------------------------------------------------- |
| Promotion argument invalid                         | `RAG_PROMOTION_ARGUMENT_INVALID`                                  |
| Promotion head stale                               | `RAG_PROMOTION_HEAD_STALE`                                        |
| Candidate not `verified/ready` or manifest differs | `RAG_PROMOTION_NOT_READY`                                         |
| Recomputed manifest differs                        | `RAG_PROMOTION_FENCE_MISMATCH`                                    |
| Delete changed current corpus coverage             | verifier error, including `RAG_STRUCTURE_VERIFY_COVERAGE_INVALID` |
| Fail argument invalid                              | `RAG_STRUCTURE_FAIL_ARGUMENT_INVALID`                             |
| Fail head/candidate/state stale                    | corresponding `RAG_STRUCTURE_FAIL_*` closed error                 |
| Failed replay changes failure code/state           | `RAG_STRUCTURE_FAIL_REPLAY_MISMATCH`                              |

### Good / Base / Bad Cases

- Good: a verified candidate whose corpus is still current recomputes the same
  manifest under the lock; D.3c may then perform the separately authorized
  cutover.
- Base: a stale verified candidate is marked `failed/failed`, identical replay
  succeeds, and a fresh `building` candidate can be allocated.
- Bad: deletion commits while promotion is waiting; the verifier sees the new
  corpus, rejects coverage, and the active generation remains unchanged.

### Tests Required

- schema tests assert the shared head lock, in-promotion verifier call, exact
  manifest fence, failure transitions, idempotent replay checks, narrow grant,
  and down-migration restoration;
- disposable integration proof must cover delete-before-promotion and a true
  concurrent lock race, asserting the closed coverage error and lock wait;
- call failure rollback twice with identical inputs, then assert
  `failed/failed`, unchanged active/head, and successful replacement allocation;
- exercise migration down/up and prove the formal database was not mutated.

### Live Proof

A disposable ACL-preserving clone rebuilt and verified the real PDF plus two
DOCX corpus. Tombstoning the PDF made the first candidate fail promotion on
current-document coverage; fail rollback and its immediate replay both
succeeded, after which a two-DOCX replacement candidate was allocated,
processed through real MinerU/Native plus Jina passage embeddings, and verified.

For the race proof, the delete transaction locked the corpus head, slept for
two seconds, and tombstoned one DOCX. Promotion started while that lock was
held, waited 1,908 ms, then recomputed coverage and failed with
`RAG_STRUCTURE_VERIFY_COVERAGE_INVALID`. The active generation and head
revision remained unchanged. The replacement then transitioned to
`failed/failed`, identical fail replay succeeded, and a new one-document
`building/building` candidate was allocated.

Migration down/up replay passed. Cleanup removed the clone, both temporary
containers, Windows result proxy, and credential-bearing environment files;
the formal database remained at migration 27 with no D.3a/D.3b function and
the original active generation.

### Wrong vs Correct

Wrong: trust the manifest frozen before a delete, or let deletion and promotion
inspect different corpus-head snapshots.

Correct: serialize both operations on the corpus head, recompute the verifier
inside promotion, fail the stale candidate audibly, and allocate a fresh
candidate without ever moving the active head.

### Rollback

Migration 032 down drops the fail function and restores migration 010's
promotion body. It does not rewrite historical candidate states. Before a
production downgrade, account for any candidate already marked failed.

## 15. G11.9D.3c Atomic Cutover and Source-Generation Rollback

### Scope / Trigger

After D.3a verification and D.3b stale-candidate fencing pass, the Go runtime
may perform the first successful promotion. The newly active structure
generation must immediately serve citation-grade Parent/Child evidence, while
one guarded operation can restore the exact generation from which that
candidate was allocated.

### Signatures

```sql
knowledge_promote_index_generation(
  index_generation_id UUID,
  expected_head_revision BIGINT,
  manifest_hash TEXT
) RETURNS BOOLEAN

knowledge_rollback_index_generation(
  active_generation_id UUID,
  target_generation_id UUID,
  expected_head_revision BIGINT,
  active_manifest_hash TEXT,
  target_manifest_hash TEXT
) RETURNS BOOLEAN
```

### Contracts

- migration 033 grants the already fenced promotion function to
  `go_api_runtime`; no second promotion implementation is introduced;
- promotion still reruns the D.3a verifier under the D.3b corpus-head lock,
  then atomically changes old `active/ready -> retired/retired`, candidate
  `verified/ready -> active/ready`, and advances both head revisions;
- rollback locks the expected active head and both generation/state rows;
- the active generation must be a D.2.3 structure rebuild whose immutable
  allocation snapshot names the requested target as `sourceGenerationId`;
- active and target manifests must match both generation and projection state;
- every current active document/version/file/content tuple must still have an
  exact published target head/materialization plus a complete Parent/Child/
  ready Jina 1024 projection;
- historical target materializations that are no longer current do not block
  rollback; existing query authorization, visibility, revision, and deletion
  fences continue to hide them;
- success atomically retires the structure generation, restores only its exact
  source generation to `active/ready`, advances the head, and cannot be replayed
  with the stale pre-rollback head revision;
- rollback is one-step recovery, not generation toggling. Re-entering the
  structure generation requires a fresh rebuild and verification.

### Validation & Error Matrix

| Condition                                            | Result                                          |
| ---------------------------------------------------- | ----------------------------------------------- |
| Invalid IDs/head/manifests                           | `RAG_GENERATION_ROLLBACK_ARGUMENT_INVALID`      |
| Active head/revision differs                         | `RAG_GENERATION_ROLLBACK_HEAD_STALE`            |
| Active generation/state/manifest differs             | `RAG_GENERATION_ROLLBACK_ACTIVE_MISMATCH`       |
| Target is not the active rebuild's source            | `RAG_GENERATION_ROLLBACK_SOURCE_MISMATCH`       |
| Target is not matching `retired/retired`             | `RAG_GENERATION_ROLLBACK_TARGET_MISMATCH`       |
| A current document lacks its exact target bytes/head | `RAG_GENERATION_ROLLBACK_COVERAGE_INVALID`      |
| Target Parent/Child/vector is incomplete             | `RAG_GENERATION_ROLLBACK_PROJECTION_INCOMPLETE` |
| A transition CAS loses                               | `RAG_GENERATION_ROLLBACK_STATE_STALE`           |

### Good / Base / Bad Cases

- Good: promote the verified structure candidate, query/cite its Parent and
  Child, then atomically restore the exact source generation.
- Base: the target contains extra historical rows for tombstoned or superseded
  documents; rollback succeeds, while query fences keep those rows invisible.
- Bad: a current target vector is missing, a document version changed, the
  target is not the allocation source, or the head revision is stale; rollback
  aborts without partially changing either generation.

### Tests Required

- schema tests assert promotion/rollback grants, source binding, coverage and
  projection fences, atomic status/head transitions, and down revocation;
- disposable integration must rebuild all current documents through real
  Native/MinerU Parse and Jina passage embeddings, verify, and promote as
  `go_api_runtime`;
- candidate and hydration queries plus a real model stream must return a `[K]`
  citation bound to the promoted generation, Parent, and Child;
- transactionally remove one target ready vector and assert rollback rejects
  it and restores the transaction; then prove successful rollback and stale
  replay rejection;
- after rollback, both direct retrieval and a second real model stream must
  cite the restored generation;
- migration down/up, full tests/build, formal-database non-mutation, and
  temporary-resource cleanup must pass.

### Wrong vs Correct

Wrong: treat every historical target materialization or old processing revision
as current coverage, making the exact previous active generation impossible to
restore even though query fences already hide stale evidence.

Correct: require exact current document bytes and queryable Parent/Child/vector
lineage, bind rollback to the candidate's source generation, and leave
visibility/revision filtering to the same runtime query authority used before
cutover.

### Live Proof

The ACL-preserving three-document clone completed three Parse and three real
Jina passage-embedding jobs on their first attempts. Verification froze 3
documents, 10 Blocks, 3 Parents, and 3 Children. Promotion as
`go_api_runtime` moved the structure generation to active at head revision 5
and retired the old generation.

Keyword retrieval returned only the new generation. A real `gpt-5.6-sol`
stream completed with `answered`, rerank `applied`, `[K1]`, and a citation bound
to the new generation's Parent and Child. Removing the restored target's ready
vector inside a transaction caused
`RAG_GENERATION_ROLLBACK_PROJECTION_INCOMPLETE`; connection rollback restored
the vector and active head.

The valid rollback returned true, restored the exact source generation at head
revision 6, and left the structure generation retained as `retired/retired`.
Direct retrieval and a second real `[K1]` stream both cited the restored
generation. Replaying the old rollback inputs failed with
`RAG_GENERATION_ROLLBACK_HEAD_STALE`.

### Rollback

Execute the guarded generation rollback before downgrading if the new
generation is active. Migration 033 down only drops the rollback function and
revokes both runtime cutover permissions; it deliberately does not rewrite
already-transitioned generation state.

## 16. Structure Chunk Profile v2

### Frozen Identity

The production candidate descriptor is immutable and registered by migration
`044`:

```text
schema version:          mm-chat.structure-chunk-profile.v2
structure profile hash: 606d6ac1cca428a05a7dccce0b172aabfba893f02431834cdc75775342db88b1
semantic profile hash:  3c17b8c1ddbed7b0a241dc43bdb24d3615526e94700c0971e585aa25519b409d
tokenizer profile hash: bdff1b0c1c8195fc2fd0a1818bac2ca66a9332a53a5cdf3d434132dff02724a0
```

The tokenizer contract is:

```text
package:                tiktoken==0.13.0
name:                   cl100k_base
revision:               openai-public-2022-12-14
normalization:          none
special-token policy:   encode_ordinary
artifact SHA-256:       223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7
vocabulary SHA-256:     d48a1992b71a810f377931afd97b5b28588e412918a3f2d9e445b019f29dc6e4
```

The packaged `.tiktoken` artifact is the only vocabulary authority. A missing,
changed, incomplete, or byte-reconstruction-inconsistent artifact fails before
chunk planning. Chat-model selection never changes this persisted projection
and never triggers knowledge reindexing.

### Automatic Routes and Bounds

The profile selects one read-only diagnostic strategy per canonical unit:

| Canonical unit                | Strategy                                             |
| ----------------------------- | ---------------------------------------------------- |
| narrative/list/quote/raw HTML | semantic hint, then sentence-recursive fallback      |
| table/sheet                   | retained header plus row groups, then token fallback |
| code                          | logical lines/regions, then token fallback           |
| JSON                          | subtree/path boundaries, then token fallback         |
| slide/shape/notes             | slide-shape boundaries, then token fallback          |
| formula                       | atomic when possible, then bounded token fallback    |
| non-indexable source block    | preserved in Canonical IR, excluded from retrieval   |

Children target `300..500` tokens around `400`, hard-stop at `650`, and reuse
up to `100` exact adjacent tokens around a `64` target. Parents target
`1200..1600` and hard-stop at `2000`. A deterministic derived-context prefix is
limited to `96` tokens and may carry a heading path, table/sheet header, JSON
path, code signature, or slide title. It is separately labeled and hash-bound;
only the original source span is quote and Citation authority.

Repeated headers, footers, watermarks, navigation, and provider-classified page
numbers remain in source artifacts and provenance but are `nonIndexable` for
Child content, Embedding, and BM25. Repetition admission combines exact text,
page position, and document frequency; no LLM deletes or rewrites source data.

### Semantic Boundary Profile

Semantic hints are ingestion-only and admitted only for an indexable narrative
unit of at least `1200` frozen-tokenizer tokens and `4..4096` sentences. BGE-M3
1024-dimensional sentence embeddings are batched within the admitted
SiliconFlow request bounds; adjacent cosine distances use
the frozen `0.85` percentile, minimum distance `0.15`, and at most `128`
boundaries. Cache identity is `(semantic profile hash, unit content SHA-256)`.
Provider timeout, quota, malformed vector, or gateway failure returns no hints
and deterministically falls back to sentence-recursive planning. A semantic
hint can select a valid boundary but can never cross structure, authorize
source text, or relax a hard token cap.

## 17. Child-First Retrieval and Shared Answer Budget

Migration `043` keeps candidate ranking on Children and extends final-authority
hydration with the complete Child text/token count plus its Parent text/token
count. Migration `045` rejoins the ready Child Search projection at that same
authority boundary. The Citation snippet, hashes, and locator remain bound to
the matched Child; the Parent locator may cover a wider source range and is not
returned as Citation authority.

Answer assembly first admits complete ranked Children. It may then expand the
top hit and other hits whose positive score is at least `0.85` of the top score,
with at most two distinct Parents per turn. Parents are deduplicated by
`ParentChunkID`, are labeled context-only, and never mint an independent
Citation. When the lane budget is tight, low-ranked Children are omitted before
any admitted block is truncated; Parent expansion is skipped when its delta
does not fit.

Knowledge and external Web evidence share one turn-local model budget:

- calculate the selected model's input budget with the existing context policy;
- subtract current System Prompt, Messages, Prompt, and a `512`-token envelope;
- cap all retrieval evidence at `40%` of the model input budget;
- when both lanes are available, reserve `60%` for Knowledge and `40%` for Web;
- when only one lane is available, it receives the full retrieval allocation;
- enforce the same ledger for native Tool results, compatibility prompts, and
  recovery fallback; Web bodies are token-trimmed rather than governed only by
  a byte ceiling.

No lane may borrow the other lane's fixed share in the mixed-source case. This
prevents one large Knowledge result or Web response from consuming the answer
window while preserving simultaneous Knowledge + Web execution.

## 18. Operator-Only Candidate Lifecycle

Migration `044` revokes begin, verify, fail, raw promotion, and rollback from
`go_api_runtime`. Migration `046` additionally revokes direct failure from
`rag_replay_operator`; the operator receives only source-text-free status,
document allocation, registered-profile begin, verification, audited
abandonment/activation, and guarded rollback gateways. Raw failure and raw
promotion are not granted.

`rag-replay generation-abandon` is dry-run by default. Execution requires the
exact Candidate UUID, verified artifact manifest, current head revision,
operator UUID, a `1..1024` UTF-8 byte reason, and both `--confirm-abandon` and
`--execute`. The database reuses the verified/ready manifest/head CAS, fixes
the failure code to `OPERATOR_ABANDONED`, leaves the Active pointer unchanged,
and appends an immutable audit. Exact replay is idempotent; a changed reason,
operator, manifest, head, or Candidate fails closed.

`rag-replay generation-verify --execute` freezes the manifest and changes only
`building/building -> verified/ready`; it never activates. First activation
requires an explicit `generation-activate --confirm-activation --execute` plus
an exact gate-report file SHA-256 and operator UUID. The immutable activation
audit records Candidate ID, previous generation, manifest hash, gate-report
hash, operator ID, and head revision before/after.

The Candidate-only gate report must contain at least 500 human-reviewed cases
split exactly `300/100/100` across Development/Validation/one-shot Holdout, at
least 50 cases for every critical format/language/query slice, the frozen
absolute retrieval/answer/citation thresholds, 100% Citation/Locator integrity, zero ACL,
deletion, secret, or unauthorized-evidence leakage, and passing latency/context
token budgets. A passing report still does not self-activate; the operator must
issue the explicit audited command.

The current Active generation must remain unchanged until that full report is
available and separately approved. Once an admitted BGE Generation has been
Active, retain exactly one complete BGE Last-Known-Good generation for guarded
compare-and-swap rollback. Historical Jina can never be a rollback target;
deletion authority continues to override rollback retention.

The executable corpus contract is `neo-chat.rag-promotion-golden.v1`. Draft
cases are legal curation artifacts but are not admitted: every promotion case
must carry a reviewer UUID and RFC3339 review timestamp, the corpus must use an
exact `60/20/20` split, and its canonical content hash must match the frozen
lifecycle record. Candidate observations bind the exact frozen corpus hash and
Generation manifest and carry one precommitted `ordinal=1` Holdout run. Active/
Jina comparisons, relative improvement, and per-slice no-regression do not
participate. `cmd/rag-eval` emits the closed v2 report schema with raw input
hashes, metric provenance, exact budgets, and zero source bodies. The
Python activation validator rechecks the complete closed shape and arithmetic;
a summary-only report, 499-case corpus, weak slice, draft review, repeated
Holdout, stale manifest, non-finite number, or sub-100% Locator/provenance/cell
rate fails closed. The curation and replay procedure is defined in
[`rag-promotion-golden-workflow.md`](./rag-promotion-golden-workflow.md).

## 19. Exact Child Locator Authority

`knowledge_locator_summary_is_valid(JSONB)` admits only the frozen
`g7.4-locator-summary.v1` shape, matching primary/fragment entries, ordered
aggregate hashes, and valid type-specific primary coordinates. Generation
verification validates Parent context locators and Child Search locators
independently; it never requires them to be byte-equal. Promotion inherits the
same fence by re-running verification, and rollback rejects any current target
Child whose ready Search projection or locator lineage is incomplete.

Final hydration must match the ready Search row through Child, Parent,
materialization, generation, collection, document/version, Search Profile,
source-span hash, chunk-profile hash, and content hash. It returns
`search.locator_summary`, never `parent.locator_summary`. A missing, malformed,
or substituted Search locator omits the reference so no Knowledge Citation or
Parent context is minted.

Native Office mapping must expose the structural view before projection:

- PPTX text fragments walk Paragraph -> Shape -> Slide and retain the stable
  Shape identity, zero-based slide index, and admitted BBox as `slide_shape`;
- XLSX row fragments retain Cell -> Sheet ancestry and every admitted A1 cell
  anchor as `sheet_range`, which projects to `sheet_cell`;
- locator selection uses `page_region`, `sheet_range`, `slide_shape`,
  `ooxml_path`, then `source_text_position`. Generic OOXML line positions must
  not mask a more exact slide or sheet locator.

## 20. Candidate Worker Image Fence

A Candidate manifest may contain projection evidence from exactly one admitted
Worker image revision. Before `generation-begin`, stop every Worker capable of
claiming Candidate parse or passage-embedding jobs, select/deploy one pinned
image, allocate the Candidate, and then start only that image. Stopping the old
Worker after allocation is insufficient because it may already lease a pending
job.

If more than one Worker image revision claims jobs for the same Candidate, the
Candidate is not certifiable even when all jobs succeed. Abandon it through
`generation-abandon` and allocate a new sequence; never repair projection rows
in place or treat a deterministic verification hash as proof of image purity.

The 2026-07-24 live replay followed this fence and verified sequence `7` twice:

```text
Candidate UUID:     53cfdad8-4e69-4d9e-a4c0-d2fcaec29696
documents:          55
blocks:             846
Parents / Children: 147 / 150
maximum Child:      397 tokens
overlapped Children: 3
artifact manifest:
7d5507b73294d5bbcb95862f858d2f9dd9ea3cc3473d078604801244d3a1de9b
```

Active remained sequence `3`; this replay is structural evidence only and is
not an activation or human-reviewed Golden result.

## 21. SiliconFlow Pro BGE Candidate Profile v3

Migration `049` registers the next Candidate-only structure and retrieval
identity without changing the Active Jina Generation:

```text
structure profile: 36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73
semantic profile:  f8de6087c6b28fe89b904549e0ddcbe4b51ebb88aecf8232ab07e6ec0d316165
provider profile:  siliconflow_bge_m3_v1
embedding model:   Pro/BAAI/bge-m3
rerank model:      Pro/BAAI/bge-reranker-v2-m3
dimensions:        1024
```

Passage and query Embedding use the exact SiliconFlow
`https://api.siliconflow.cn/v1/embeddings` response contract. Rerank uses the
exact `https://api.siliconflow.cn/v1/rerank` contract. Reusable credentials
remain inside the Go Provider Gateway; the Python Worker receives only the
private gateway URL and internal token. The admitted governance profile is
`CN`, request-scoped, training-disabled, and provider-request-ephemeral.

The Worker resolves the Embedding model from the immutable target Generation
before staging passage vectors. Query-time Go code resolves the Active
Generation/Search Profile before calling a provider, authorizes that exact
model identity, and searches only rows carrying the same Generation and Search
Profile IDs. If the pointer changes after Embedding, the database raises
`RAG_RETRIEVAL_PROFILE_CHANGED`; Go resolves and retries once. Provider failure
may use the same fenced BM25 lane but may never call Jina and apply its vector
to a BGE index. Rerank resolves from the Generation carried by hydrated
evidence, not from a later Active-pointer read.

Matching `1024` dimensions are not vector-space compatibility. Jina and BGE
have separate partial HNSW indexes and exact profile admission. Stable lexical
and hybrid entrypoints retain the pre-cutover `legacy` behavior; the fenced
PG17 branch is reachable only while `pg17_bm25_pgvector_v1` is selected. The
operator diagnostic signature is also Active Generation/Search Profile bound
and returns references, ranks, and scores only.

Migration `049` performs no Activation or Holdout. Migration `050` then removes
all Jina runtime authority without moving Active or Candidate. A complete BGE Candidate
must be rebuilt from source, reconciled to a fenced corpus revision, verified,
and evaluated through the frozen human-reviewed gates before an explicit
operator cutover. Its down migration refuses atomically with
`RAG_SILICONFLOW_ROLLBACK_REQUIRES_BGE_PURGE` while any BGE profile or
projection remains.

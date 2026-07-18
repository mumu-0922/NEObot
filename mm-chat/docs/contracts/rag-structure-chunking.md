# RAG Structure-Aware Chunk Planning Contract

## 1. Scope / Trigger

This contract covers G11.9D.1 deterministic planning, G11.9D.2.1 Native
projection, G11.9D.2.2 admitted MinerU page-element projection,
G11.9D.2.3a candidate-generation allocation, and G11.9D.2.3b leased candidate
parse projection, G11.9D.2.3c real passage-embedding completeness, and
G11.9D.3a generation verification. These slices may publish candidate
materializations and transition only the candidate to `verified/ready`. They
must not switch the active Index Generation; deletion/race fencing and atomic
promotion remain D.3b/D.3c.

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
- Planning is pure and byte-deterministic: no filesystem, network, database,
  clock, randomness, provider, tokenizer download, or global mutable state.
- The current conservative token estimate is `ceil(utf8_bytes / 4)`. It is a
  versioned planning estimate, not a claim about Jina/provider tokenization.
- Typical Children target 300–500 tokens with a 400-token center and hard cap
  650. Adjacent children reuse exact atom ranges for up to 100 tokens, targeting
  about 64.
- Parents target 1,200–1,600 tokens with a hard cap of 2,000. A Parent never
  crosses a `heading_path` change.
- Table rows, code, formula, heading, and other protected units remain atomic
  while they fit the 500-token target maximum. Oversized protected units split
  only at UTF-8-safe scalar boundaries.
- The planner caps one unit at 4 MiB, the document at 32 MiB, and unit count at
  100,000. Runtime admission may be stricter.
- Output references preserve source ordering and Parent containment. Overlap
  ranges must be byte-identical ranges present in the immediately preceding
  sibling Child.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Empty/non-tuple or more than 100,000 units | `StructureChunkingError` |
| Non-contiguous unit ordinal | `StructureChunkingError` |
| Unknown structural kind | `StructureChunkingError` |
| Empty, NUL, invalid scalar, or oversized text | `StructureChunkingError` |
| Invalid/oversized heading path | `StructureChunkingError` |
| No UTF-8-safe forward boundary | `StructureChunkingError` |
| Parent or Child exceeds its hard token cap | `StructureChunkingError` |

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
  formula/equation, footnote, header, and footer map to Canonical text blocks;
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

Tests cover the frozen synthetic MinerU heading/text/table/formula corpus,
page-BBox projection, deterministic replay, schemas, Postgres DTO projection,
long multilingual UTF-8/overlap behavior, and the observed real-provider
`pdf_info[]` archive shape. Unsupported provider shape must fail closed.

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

| Condition | Error |
| --- | --- |
| Null ID, invalid hash, non-array/empty allocations | `RAG_STRUCTURE_REBUILD_ARGUMENT_INVALID` |
| No active generation | `RAG_STRUCTURE_REBUILD_ACTIVE_GENERATION_MISSING` |
| Existing building/verified candidate | `RAG_STRUCTURE_REBUILD_CANDIDATE_EXISTS` |
| Missing active Search Profile | `RAG_STRUCTURE_REBUILD_ACTIVE_PROFILE_MISSING` |
| Count, uniqueness, or exact document set mismatch | `RAG_STRUCTURE_REBUILD_ALLOCATION_COVERAGE_INVALID` |
| Invalid allocation UUID/hash shape | `RAG_STRUCTURE_REBUILD_ALLOCATION_INVALID` |
| Document or latest parse authority unavailable | `RAG_STRUCTURE_REBUILD_DOCUMENT_INVALID` |

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

| Condition | Result |
| --- | --- |
| Null ID or zero lease token | `RAG_PARSE_CHUNK_PROFILE_ARGUMENT_INVALID` |
| Wrong/expired lease, non-parse job, legacy-unbound job, mismatched binding, non-staging materialization, or unavailable generation | `RAG_PARSE_CHUNK_PROFILE_MISSING` |
| Bound hash is neither baseline nor shared structure | `NATIVE_PARSER_CHUNK_PROFILE_UNSUPPORTED` |
| Processor is neither admitted MinerU nor Native | `NATIVE_PARSER_AUTHORITY_UNSUPPORTED` |
| Real MinerU layout loses/changes text-bearing shape | `MINERU_STRUCTURE_ARTIFACT_INVALID` |

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

| Condition | Result |
| --- | --- |
| Missing/stale lease or substituted materialization | `RAG_STALE_JOB_LEASE` or materialization-missing error |
| Empty/duplicate/malformed candidate rows | fixed embedding-candidate error |
| Provider count, Child ID, dimension, or finite-value mismatch | fixed Jina/handler vector error |
| Stage target does not match immutable Child/search lineage | `RAG_PASSAGE_EMBEDDING_TARGET_MISSING` |
| Ready search count differs from Child count | `RAG_SEARCH_PROJECTION_INCOMPLETE` |
| Source authority changed before publish | `RAG_EMBEDDING_COMPLETION_AUTHORITY_STALE` |

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

| Condition | Result |
| --- | --- |
| Invalid ID/head/hash argument | `RAG_STRUCTURE_VERIFY_ARGUMENT_INVALID` |
| Head revision stale, missing active head, or candidate is active | `RAG_STRUCTURE_VERIFY_HEAD_STALE` |
| Candidate/status missing | `RAG_STRUCTURE_VERIFY_CANDIDATE_MISSING` |
| State/readiness/outbox floor inconsistent | `RAG_STRUCTURE_VERIFY_STATE_INVALID` |
| Index/Search/Jina/shared profile mismatch | `RAG_STRUCTURE_VERIFY_PROFILE_MISMATCH` |
| Exact current document tuple coverage differs | `RAG_STRUCTURE_VERIFY_COVERAGE_INVALID` |
| Latest Parse/Embedding job incomplete | `RAG_STRUCTURE_VERIFY_JOBS_INCOMPLETE` |
| Published document heads incomplete | `RAG_STRUCTURE_VERIFY_HEADS_INCOMPLETE` |
| Parser artifacts/Blocks incomplete | `RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE` |
| Parent/Child/vector/locator lineage incomplete | `RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE` |
| Verified replay recomputes different manifest/counts | `RAG_STRUCTURE_VERIFY_REPLAY_MISMATCH` |

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

Deletion/race fencing, failed-candidate rollback, atomic promotion, and live
citations remain G11.9D.3b/D.3c.

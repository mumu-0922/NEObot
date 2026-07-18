# RAG Structure-Aware Chunk Planning Contract

## 1. Scope / Trigger

This contract covers G11.9D.1 deterministic planning, G11.9D.2.1 Native
projection, G11.9D.2.2 admitted MinerU page-element projection, and the
G11.9D.2.3a candidate-generation rebuild allocator. The allocator may persist
only a non-active generation and its staging work. It does not yet replace a
production gateway, call Jina, verify a candidate, or switch the active Index
Generation. Those promotion steps remain D.2.3 and D.3.

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
structure contract is `middle_json.pages[].elements[]` with contiguous
zero-based page indexes, positive page geometry, bounded page BBoxes, and
closed text-bearing kinds.

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

Tests cover the frozen synthetic MinerU heading/text/table/formula corpus,
page-BBox projection, deterministic replay, schemas, Postgres DTO projection,
and long multilingual UTF-8/overlap behavior. Real-provider archive replay is
a D.2.3 prerequisite; unsupported provider shape must fail closed.

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

# RAG Structure-Aware Chunk Planning Contract

## 1. Scope / Trigger

This contract covers G11.9D.1: deterministic planning of structure-aware Parent
and Child chunks from already validated, source-derived text units. It does not
yet replace the Native/MinerU baseline, persist new materializations, call Jina,
or switch the active Index Generation. Those promotion steps remain D.2/D.3.

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

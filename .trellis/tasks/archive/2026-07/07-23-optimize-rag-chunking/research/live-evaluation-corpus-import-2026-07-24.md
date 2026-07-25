# Live synthetic evaluation corpus import — 2026-07-24

## Scope and guardrail

This operation created a separate, reversible personal Knowledge collection for
Golden-question curation. Every source and expected fact is synthetic and
`draft`-only. The corpus is not human-reviewed promotion evidence and no
generation activation command was executed.

## Frozen source corpus

```text
collection:
RAG Golden Evaluation Corpus — Synthetic Draft

collection UUID:
516c803c-1e59-46f6-8093-d639655bcb2d

source manifest SHA-256:
e014f999f48a3a7cbb0d10fa9f60d8404752bb026ba21ae70ea121f5a6d230df

documents / unique content hashes: 50 / 50
PDF / DOCX / PPTX / XLSX / Markdown JSON+code: 10 / 10 / 10 / 10 / 10
Chinese / English: 25 / 25
expected facts: 500 total, 10 per document
review state: draft only
promotionEligible: false
```

The deterministic generators are:

```text
research/generate-evaluation-corpus.py
research/generate-evaluation-office.cjs
```

The exact import receipt, including source, File, and Document UUID bindings,
is stored separately at:

```text
research/live-evaluation-corpus-import-2026-07-24.json
receipt SHA-256:
4c727519ef651a1101879c11a8f7de9fed400035351bfdceea49d852a3635645
```

## Local source validation

- Two complete generator replays produced 51 byte-identical files, including
  the manifest.
- All 50 sources passed the strict format Router.
- All 40 DOCX/PPTX/XLSX/Markdown sources passed the current Native Parser.
- Native structure totals included 30 Slides, 150 Shapes, 30 Sheets, 40
  Tables, 570 Table Cells, and 30 Code nodes.
- Every PDF contained exactly four pages and all ten evidence anchors were
  extractable.

## Live API and worker result

The normal public API path was used for every source:

```text
POST /v1/files
POST /v1/knowledge/collections/{collectionId}/documents
```

The server returned 50 distinct File UUIDs and 50 distinct Document UUIDs.
Every upload response SHA-256 matched the frozen local manifest. Final worker
state was:

```text
Documents active:                    50 / 50
Document Versions active:            50 / 50
Parse jobs succeeded:                50 / 50
Passage embedding jobs succeeded:    50 / 50
Failed jobs:                          0
Published projection heads:          50 / 50
Ready Child Search projections:      50 / 50
```

## Generation boundary

The production pointer did not move:

```text
active generation sequence:        3
active generation UUID:            46a1c7bb-44ed-4868-9d61-edd557f9d3f0
active chunk profile:              byte-window baseline
head revision:                     4
corpus projection revision:        78

existing Structure Candidate sequence: 4
existing Candidate UUID:                a4839e8b-6bb6-41a4-93eb-b94e33f7130f
existing Candidate document count:      5
```

The active byte-window path intentionally emitted one Parent and one Child per
new source, with a single `line_range` paragraph Block. This proves upload,
parse, embedding, and active publication only; it is not evidence that the
Structure Candidate handled the new formats. The previously verified Candidate
is now corpus-stale and must be replaced or rebuilt against revision 78 before
Golden observations are captured.

## Rollback surface

The corpus is isolated by one collection UUID. An intentional rollback can
delete that collection through the public Knowledge API and then wait for all
generation-bound purge jobs. Do not delete individual files directly while
their Documents remain active.

## Follow-up verification and draft queue

The stale sequence `4` Candidate was not reused. After correcting MinerU nested
table parsing, PPTX/XLSX locator authority, and the Worker image-allocation
order, sequence `7` rebuilt and deterministically verified all 55 current
Documents. Exact Candidate evidence is recorded in
[`live-candidate-verification-2026-07-24.md`](./live-candidate-verification-2026-07-24.md).

The source manifest and this exact import receipt now deterministically produce
500 curation cases through:

```text
generate-promotion-draft-queue.py
promotion-golden-synthetic-draft-2026-07-24.json
promotion-curation-queue-synthetic-draft-2026-07-24.json
```

The queue is exactly `300/100/100` Development/Validation/Holdout, has 50
table-exact cases, and binds every question to one `sourceId:factAnchor`, source
SHA-256, File UUID, and Document UUID. All 500 review records remain `draft` and
`promotionEligible=false`; no human review or activation is implied.

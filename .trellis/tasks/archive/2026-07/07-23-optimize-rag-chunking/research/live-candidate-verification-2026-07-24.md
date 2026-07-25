# Live Structure Candidate Verification — 2026-07-24

## Scope

This proof applied migration head `046`, rebuilt the complete current corpus
with the registered Structure Profile v2, and verified Candidate sequence `7`
twice. It did not execute `generation-activate` and did not move the Active
corpus head.

The rebuilt corpus contains the five pre-existing documents plus the dedicated
50-document synthetic evaluation collection. Synthetic sources and generated
questions remain draft-only and are not promotion evidence.

## Runtime findings and fixes

### MinerU nested tables

The live MinerU provider represents some tables as:

```text
table -> blocks -> table_caption / table_body -> lines -> span.html
```

The previous mapper inspected only ordinary span `content`, so the nested table
body could disappear. The mapper now recursively reads the nested block tree
and passes only `span.type=table` HTML through a bounded `HTMLParser`. It emits
cell text with deterministic row and column separators, decodes character
references, escapes source `|` characters, rejects malformed/empty tables, and
never executes or retains HTML.

### Native Office locator authority

- PPTX Paragraph fragments walk to their owning Shape and Slide and emit a
  `slide_shape` view with the stable shape identity and admitted geometry.
- XLSX Row fragments retain every Cell/Sheet anchor and emit `sheet_range`
  views, which project to `sheet_cell` Citation locators.
- Projection selects native structural authority in this order:
  `page_region`, `sheet_range`, `slide_shape`, `ooxml_path`, then generic
  `source_text_position`. XML line positions can no longer mask exact slide or
  sheet coordinates.

### Candidate worker image fence

Candidate sequence `6` exposed an operational race: an old Worker and the new
Worker image both claimed jobs allocated to the same Candidate. A Candidate is
immutable evidence, so mixed image revisions cannot be repaired or certified.

The replay order is now:

```text
stop all Candidate Workers
deploy/select one pinned Worker image
generation-begin
start only the pinned Worker
drain and verify
```

Stopping Workers after allocation is too late because a pending job may already
be leased by the old image. A mixed-image Candidate must be abandoned through
the audited gateway and rebuilt under a new sequence.

## Superseded Candidates

All three stale Candidates were abandoned through migration `046` with
operator UUID `1f53685f-b00d-4d8f-8d0d-a7f521b5246b`:

```text
sequence 4  a4839e8b-6bb6-41a4-93eb-b94e33f7130f
reason      predates the 50-document import

sequence 5  a70c02e6-49ea-412c-9a3b-ff1b6b147eb6
reason      lacked PPTX/XLSX locator authority

sequence 6  e477bf65-b36d-450a-80a1-d1cddad3b456
reason      mixed Worker image revisions
```

The abandonment gateway used exact Candidate/manifest/head compare-and-swap,
fixed the failure code to `OPERATOR_ABANDONED`, wrote immutable audits, and
left Active unchanged.

## Final verification evidence

```text
migration head:                 46
active generation:              46a1c7bb-44ed-4868-9d61-edd557f9d3f0
active generation sequence:     3
active status:                  active / ready
active profile:                 old byte-window baseline
head revision:                  4
corpus projection revision:     243

candidate generation:           53cfdad8-4e69-4d9e-a4c0-d2fcaec29696
candidate generation sequence:  7
candidate status/readiness:      verified / ready

artifact manifest SHA-256:
7d5507b73294d5bbcb95862f858d2f9dd9ea3cc3473d078604801244d3a1de9b

verification report SHA-256:
328b25a7485bf3d567ff86777fc59a4f04612c0e8490d0d131fbc1f275362219

documents / blocks / parents / children: 55 / 846 / 147 / 150
failed latest Candidate jobs:             0
maximum Child tokens:                   397
Children with exact adjacent overlap:     3
```

Two consecutive `generation-verify --execute` calls returned the exact same
manifest, report hash, counts, and ready state.

## Structural and locator coverage

```text
PDF page_bbox Blocks:        129 / 129
PPTX slide_shape Blocks:     190 / 190
XLSX sheet_cell Blocks:      130 / 130
XLSX table Blocks:           130
Markdown code Blocks:         30

Child Citation locators:
line_range                   103
page_bbox                     27
sheet_cell                    10
slide_shape                   10
total                        150
```

This proves all representative PDF, PPTX, XLSX, and Markdown/code lanes retain
their intended structural authority in the verified Candidate. It does not
turn synthetic facts into reviewed Golden evidence.

## Runtime and quality gates

```text
Backend go vet ./...:                  passed
Backend go test ./...:                 passed
RAG ruff check:                        passed
RAG ruff format --check:               passed
RAG mypy src tests/support:            passed
RAG pytest:                            1762 passed / 7 skipped
Focused migration/role proof:          passed
Consecutive Candidate verification:    deterministic
```

Role proof at migration head `046`:

```text
rag_replay_operator audited abandon: true
rag_replay_operator raw fail:        false
go_api_runtime audited abandon:      false
go_api_runtime raw fail:             false
```

## Activation boundary

Active remains generation sequence `3` on the byte-window profile. Candidate
sequence `7` is verified but not promotion-eligible. Five hundred cases now
carry recorded case-by-case human review, but no Active/Candidate observation
pair or one-shot Holdout comparison has run. Activation remains forbidden until
the frozen Golden Gate passes and an operator separately issues the audited
activation command.

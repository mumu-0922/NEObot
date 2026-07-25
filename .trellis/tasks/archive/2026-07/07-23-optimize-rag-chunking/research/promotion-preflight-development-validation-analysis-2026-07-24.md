# Development/Validation promotion preflight analysis — 2026-07-24

## Boundary

- Splits: Development + Validation only (`400` cases).
- Holdout: `not_executed`.
- Activation: not executed.
- Active Generation: `46a1c7bb-44ed-4868-9d61-edd557f9d3f0`.
- Candidate Generation: `53cfdad8-4e69-4d9e-a4c0-d2fcaec29696`.
- Candidate manifest:
  `7d5507b73294d5bbcb95862f858d2f9dd9ea3cc3473d078604801244d3a1de9b`.

## First complete capture

Artifact:
`promotion-preflight-development-validation-2026-07-24.json`

SHA-256:
`e4167b8ba7c8d2a475f1c89ffcb7ab0efdc6b1ee784fe5d465e5f6f0578b0194`

The capture is complete and contains independent Active/Candidate capture
UUIDs, `400` observations per profile, zero leakage, no Holdout execution, and
`promotionEligible=false`.

```text
Active quality score                 0.7763154649
Candidate quality score              0.8729155076
Aggregate improvement                +0.0966000427 (passed)
Candidate average context tokens     1733.5775 / 4096 (passed)
Candidate P95 latency                3049ms / 1000ms (failed)
Candidate table exact answer         1.0
Candidate provenance/cell lineage    1.0
```

The initial no-regression check failed only for `json_code`, `short_fact`, and
`text_markdown_docx`.

## Confirmed evaluator defects

The apparent slice regressions were not Candidate source or lineage failures.
The structure-aware Candidate correctly separated headings, configuration
blocks, and direct answer text. Some resulting Child chunks no longer carried
the synthetic `Fxx` marker even though they came from the curator-bound source
document and contained the exact reviewed answer.

The v1 capture mapper therefore emitted opaque `chunk:<uuid>` IDs and counted
them as irrelevant. It also counted two Candidate citations for the same
reviewed semantic fact when one cited Child carried the marker and another
curator-bound Child carried the direct answer. Active byte windows happened to
retain both in one chunk and avoided that penalty.

Separately, all `25` apparent answer failures were Chinese renderings of ISO
dates such as `2026年8月22日` versus reviewed answers such as `2026-08-22`.
Retrieval and Citation evidence were correct; the v1 exact-answer normalizer
did not canonicalize the equivalent date forms.

Capture policy v2 fixes both defects without broadening source authority:

- an anchorless Child maps to a reviewed evidence ID only when its current
  Document maps to the exact curator-bound Source ID and its source text
  contains the complete reviewed answer;
- explicit `Fxx` anchors retain the existing mapping path;
- duplicate Children supporting the same reviewed semantic fact collapse to
  one evidence ID;
- Chinese/ISO numeric date forms compare through canonical date components;
- the answer prompt requests the smallest sufficient directly supporting
  Citation set.

Wrong-document text remains ineligible even if it repeats the same answer.
Filename metadata remains non-evidence and is never used by this mapping.

## v2 live smoke results

Capture version: `neo-chat.rag-promotion-capture.v2`  
Scoring policy: `synthetic-curator-bound-evidence-v2`

All four representative Candidate smokes scored `1.0`, retained
`promotionEligible=false`, kept Holdout `not_executed`, and passed locator,
provenance, cell-lineage, and leakage checks:

- Chinese ISO-date rendering: `rageval-pdf-zh-01-f05`.
- Anchorless JSON/config support: `rageval-code-zh-05-f08`.
- Split DOCX title/overview support: `rageval-docx-zh-04-f01`.
- Source-name-routed cell evidence: `rageval-xlsx-zh-01-f03`.

The XLSX Candidate remained strictly better than Active because Candidate
retained exact `sheet_cell` lineage while Active did not.

## Latency boundary finding

The v2 capture now records a per-case latency breakdown. A cooled, single-case
XLSX replay produced:

```text
Shared EmbedQuery                   4555ms
Candidate FetchCandidates             65ms
Candidate HydrateEvidence              91ms
Candidate Rerank                       339ms
Candidate profile total               5051ms
Frozen P95 budget                     1000ms
```

Artifact:
`promotion-preflight-v2-xlsx-cooled-latency-2026-07-24.json`

SHA-256:
`c607a97de05b3ef95713d5e2dbd4c57273d29bb4d0c03cb7d90504336daa145d`

The source-name SQL lane and Candidate hydration are not the latency blocker.
The shared external Jina query embedding dominates even after a cooldown; a
prior quota/retry sample measured `4989ms` in the same stage. The `1000ms`
value was seeded by the draft generator before Development/Validation
calibration and is not achievable by the observed fixed external provider
pipeline. Because criteria are frozen into the reviewed Golden hash, changing
the budget requires an explicit new review/freeze cycle before any full rerun
can become promotion evidence.


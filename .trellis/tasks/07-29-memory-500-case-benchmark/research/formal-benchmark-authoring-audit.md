# Formal Memory benchmark authoring audit

Date: 2026-07-29

## Existing executable boundary

Neo Chat already has a strict offline evaluator under
`mm-chat/backend/internal/memoryeval` and the `cmd/memory-eval` command. It can:

- strictly decode Golden and observation JSON;
- calculate a canonical freeze hash;
- admit only an exact frozen 500-case `300/100/100` corpus;
- enforce critical-slice coverage and slice semantics;
- bind fixture, Golden, profile, raw observation, and precommitted Holdout IDs;
- calculate quality, latency, token, cost, and zero-tolerance authority gates;
- publish a failed or passing report exclusively without changing a reader.

The checked-in draft contains only ten cases. The evaluator contract explicitly
does not create the external synthetic fixture manifest, generate candidate
cases, collect real human review, or produce Native reader observations.

## Artifact gap

The formal chain needs four artifacts before a report is meaningful:

1. a general synthetic Memory fixture manifest containing fact/event content,
   scope, temporal state, ownership aliases, and rejected/deleted sentinels;
2. a Golden case corpus containing only queries, opaque logical IDs, expected
   authority, exclusions, split/slices, and review records;
3. immutable profile-specific observations captured from a real offline Native
   reader pipeline; and
4. the exclusive evaluator report.

The PR13 Hindsight manifest proves a useful fixture shape, but its schema and
profile semantics are Hindsight-specific. Reusing that schema as the permanent
formal benchmark authority would couple Native evaluation to a deleted external
engine. A generic `neo-chat.memory-benchmark-fixtures.v1` artifact should own the
formal fixture content instead.

## Authoring approaches

### A. Deterministic candidate pool plus case-by-case review (recommended)

- Generate approximately 600-650 synthetic candidates from versioned scenario
  templates and a fixed seed.
- Stratify templates across language, scope, temporal correction, negatives,
  untrusted/secret rejection, deletion, fallback, and multi-hop behavior.
- A human accepts, edits, or rejects each candidate through a local review
  workflow; the tool never fills reviewer identity or time automatically.
- Select exactly 500 accepted cases, freeze order/splits/hashes, then precommit
  the Holdout UUID.

This satisfies the current contract, leaves room to reject weak duplicates, and
keeps generated provenance reproducible. Its cost is genuine manual review.

### B. Generate exactly 500 and batch-approve groups

This is faster, but a single bad or duplicate row forces in-place repair and
refreeze. Batch approval also weakens the case-by-case evidence requirement and
is not suitable for formal promotion without an explicit contract change.

### C. Fully machine-authored frozen corpus

This can be useful as a draft or continuous smoke suite, but it cannot become
formal evidence. An assistant/model is not the required human reviewer, and
machine-generated reviewer UUIDs/timestamps would be fabricated authority.

## Recommended workflow

```text
versioned template catalog + fixed seed
  -> oversized draft candidate fixture/Golden pair
  -> strict local validation + duplicate/coverage diagnostics
  -> human accept/edit/reject per case
  -> exact 500 selection with 300/100/100 split
  -> independent review audit
  -> frozen fixture hash + Golden canonical hash + Holdout UUID
  -> Development/Validation observations
  -> one Holdout observation assembly
  -> exclusive report
```

The review surface should optimize operator effort but must not turn a UI action
such as "approve all" into 500 fabricated reviews. Resume/idempotency, immutable
case IDs, content hashes, and explicit edit history matter more than visual
complexity.

## Decisions that require product input

The technical gate already determines that every case needs authentic human
review. The remaining preference is the operator workflow: whether the sole
user will personally review all cases using a local tool, or whether this task
should stop at a machine-generated non-promotional draft for later review.


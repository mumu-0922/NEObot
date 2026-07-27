# Memory v2 benchmark contracts

## 1. Scope / Trigger

Apply this contract when changing `internal/memoryeval`, `cmd/memory-eval`, a
Memory benchmark fixture/observation producer, Memory retrieval ranking,
current-fact selection, prompt Memory injection, Memory Provider egress, or a
Memory reader promotion gate.

PR1 is offline-only. It adds no API route, database object, Provider call,
feature flag, or active-reader mutation. The complete operator workflow is
documented in
`mm-chat/docs/contracts/memory-benchmark-workflow.md`.

## 2. Signatures

The versioned artifacts are:

```text
neo-chat.memory-benchmark-golden.v1
neo-chat.memory-benchmark-observations.v1
neo-chat.memory-benchmark-report.v1
neo-chat.memory-benchmark-evaluator.v1
```

Draft/freeze validation:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden <draft-or-pre-frozen.json> \
  -print-freeze-hash \
  [-pretty]
```

Formal evaluation:

```bash
cd mm-chat/backend
go run ./cmd/memory-eval \
  -golden <frozen-500-case.json> \
  -observations <ordered-observations.json> \
  -output <new-exclusive-report.json> \
  [-pretty]
```

The Go entrypoints are:

```go
memoryeval.DecodeGoldenSet(io.Reader) (memoryeval.GoldenSet, error)
memoryeval.DecodeObservationSet(io.Reader) (memoryeval.ObservationSet, error)
memoryeval.GoldenContentSHA256(memoryeval.GoldenSet) (string, error)
memoryeval.ValidateGoldenAdmission(memoryeval.GoldenSet) error
memoryeval.Evaluate(memoryeval.EvaluationInput) (memoryeval.Report, error)
```

## 3. Contracts

- Input is synthetic-only and declares an explicit
  `promotionEligible=false`; omitted and `true` are both invalid.
- Golden artifacts contain opaque Memory IDs and aliases, not Memory bodies,
  chat transcripts, credentials, embeddings, or sensitive facts.
- Strict decoding rejects inputs over 64 MiB, duplicate JSON keys, unknown
  fields, trailing values, padded/control identifiers, invalid enums, and
  inconsistent ranking stages.
- Frozen admission requires exactly 500 human-reviewed cases, exact
  `300/100/100` Development/Validation/Holdout counts, at least 50 cases in
  every `memoryeval.CriticalSlices()` slice with at least `30/10/10`
  Development/Validation/Holdout coverage, review times no later than freeze,
  a fixture-manifest SHA-256, a precommitted Holdout UUID, and a matching
  canonical frozen-content SHA-256.
- Observations repeat the frozen/fixture bindings, name an immutable profile
  configuration SHA-256, use Candidate limit 20 and Final limit 5, preserve
  Golden order, and carry the precommitted Holdout UUID at ordinal one.
- `finalMemoryIds` is a subset of `candidateMemoryIds`;
  `injectedMemoryIds` is a subset of `finalMemoryIds`.
  `persistedMemoryIds` and `providerSentMemoryIds` are separate authority
  surfaces and must not be hidden behind self-reported booleans.
- v1 criteria are exact: Candidate Recall@20 `>=0.95`, Final Recall@5
  `>=0.90`, current-fact accuracy `>=0.95`, false injection `<=0.02`, P95
  `<=900ms`, P99 `<=1500ms`, 2-second cutoff, average/maximum prompt Memory
  `<=600/900` tokens, Provider cost ratio `<=0.15`, and zero authority leaks.
- Report failure strings and slice processing are sorted/deterministic.
  nDCG@5/MRR@5 are diagnostic until the same frozen corpus has baseline and
  candidate reports.
- The report is published through a same-directory temporary file plus an
  exclusive hard link. Existing output is never overwritten. A failed gate
  still publishes its report before returning non-zero.
- A passing report is evidence only. Evaluation never changes Memory Use/Learn,
  migration state, reader pointers, workers, or Hindsight.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Draft omits `promotionEligible=false` | Reject before hashing. |
| Golden contains real/sensitive-data policy flags | Reject. |
| Duplicate/unknown/trailing JSON or input over 64 MiB | Reject before typed evaluation. |
| Corpus is not exactly 500 or split is not `300/100/100` | Reject frozen admission. |
| Critical slice has fewer than 50 cases, lacks `30/10/10` split coverage, or lacks matching semantic exclusions | Reject. |
| Review is draft, malformed, or later than freeze | Reject. |
| Frozen content, fixture manifest, raw file, profile, or Holdout binding drifts | Reject. |
| Observation order differs or a case is missing/unknown | Reject. |
| Final/injected stage contains an ID not present in its parent stage | Reject. |
| Cross-user, deleted, secret, untrusted, or unauthorized Provider ID reaches a forbidden surface | Produce a failing zero-tolerance report. |
| Quality/latency/token/cost criterion fails | Produce a failing report at the new path, then return non-zero. |
| Output already exists | Refuse without changing existing bytes. |

## 5. Good / Base / Bad Cases

- **Good**: a synthetic, reviewed, hash-bound 500-case corpus produces ordered
  observations for one precommitted Holdout and an exclusive report whose raw
  hashes can be independently replayed.
- **Base**: the checked-in ten-case draft template strictly decodes and emits a
  freeze hash with `promotionEligible=false`, but frozen admission refuses it.
- **Bad**: generate 500 machine-authored rows, copy a fake reviewer UUID/time,
  omit privacy flags, score them as Holdout, overwrite a prior report, or let a
  passing evaluator change the active reader.

## 6. Tests Required

- Strict decoder tests: duplicate keys, unknown fields, trailing JSON, explicit
  promotion denial, bounded input, enums, identifiers, and stage subsets.
- Golden tests: checked-in draft validation/non-admission, exact count/splits,
  every critical slice, semantic slice labels, human review, timestamps, and
  frozen hash drift.
- Binding tests: raw hashes, fixture hash, ordered exact case set, precommitted
  Holdout ID, ordinal one, and freeze/capture time window.
- Metric tests: Recall@20, Final Recall@5, current-vs-superseded, false
  injection, nDCG/MRR, P95/P99, hard cutoff, average/max tokens, and exact 15%
  integer cost boundary without overflow.
- Safety tests: cross-user/out-of-scope, deletion, secret persistence/exposure,
  untrusted-source persistence, and forbidden Provider egress are all zero.
- Command tests: freeze output remains non-promotional, argument modes are
  exclusive, report mode is `0600`, and an existing output remains byte-identical.
- Run `go test -race ./internal/memoryeval ./cmd/memory-eval`, `go test ./...`,
  and `go vet ./...` from `mm-chat/backend`.

## 7. Wrong vs Correct

### Wrong

```text
machine-generate 500 cases with human_reviewed labels
-> run Holdout repeatedly
-> overwrite report.json until passed
-> enable the candidate reader
```

This fabricates authority, leaks Holdout into tuning, destroys failed evidence,
and couples scoring to activation.

### Correct

```text
synthetic draft (promotionEligible=false)
-> case-by-case review
-> freeze exact content + precommit Holdout UUID
-> complete Dev/Validation
-> one ordered Holdout
-> publish a new exclusive report
-> request a separate reader-promotion decision
```

The evaluator proves the artifact chain and metrics while remaining unable to
change production.

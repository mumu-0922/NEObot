# memoryeval

`memoryeval` is the offline, no-network contract for Neo Chat Memory benchmark
artifacts. It validates synthetic Golden corpora and captured observations,
binds them to immutable hashes, and produces deterministic quality, latency,
token, cost, and authority-safety results.

Formal human-reviewed Golden evaluation and machine-reviewed regression are
separate admission lanes. They share scoring internals, but not schemas,
bindings, reports, Holdout semantics, or promotion claims.

The package exists so Memory v2 readers cannot be promoted on ad hoc examples
or vendor-reported scores. It does not import `usermemory`, connect to
PostgreSQL, call a Provider, or select the production Memory reader.

## Responsibilities

- Strictly decode versioned JSON with unknown-field, duplicate-key, trailing-
  value, and 64 MiB input rejection.
- Validate synthetic-only data policy, case semantics, review attestations,
  exact 500-case `300/100/100` splits, and minimum critical-slice coverage.
- Freeze the canonical Golden content hash without treating a draft or hash as
  promotion evidence.
- Require an ordered, single-run Holdout observation set bound to the Golden
  hash, fixture manifest, profile configuration, and raw input hashes.
- Score Recall@20, Final Recall@5, current-fact accuracy, false injection,
  nDCG/MRR diagnostics, latency, prompt tokens, Provider cost, and zero-
  tolerance authority leaks.
- Strictly admit a bound, passed machine semantic audit for the 500-case
  `machine_reviewed_regression` corpus while rejecting every human attestation,
  frozen lifecycle, or simulated Holdout run in that lane.

## Usage

Use the operator command from `mm-chat/backend/`:

```bash
go run ./cmd/memory-eval \
  -golden /secure/eval/memory-golden.json \
  -print-freeze-hash \
  -pretty

go run ./cmd/memory-eval \
  -golden /secure/eval/memory-golden.json \
  -observations /secure/eval/memory-observations.json \
  -output /secure/eval/memory-report.json \
  -pretty

go run ./cmd/memory-eval \
  -regression-corpus ../data/memory-benchmark/v2-regression/corpus.json \
  -regression-audit ../data/memory-benchmark/v2-regression/audit.json \
  -observations /secure/eval/memory-regression-observations.json \
  -output /secure/eval/memory-regression-report.json \
  -pretty
```

The report path is exclusive and is never overwritten. A failing gate still
publishes its complete report before the command returns a non-zero status.
Regression output always declares `corpusClass=machine_reviewed_regression`,
`admissionMode=regression_only`, and `promotionEligible=false`. Its `holdout`
split is visible stratification, not the formal one-shot Holdout.

## Public API

| API | Purpose |
| --- | --- |
| `DecodeGoldenSet` | Strictly decode and structurally validate a draft or frozen Golden corpus. |
| `DecodeObservationSet` | Strictly decode and structurally validate an observation set. |
| `GoldenContentSHA256` | Compute the canonical self-field-cleared freeze hash. |
| `NewFreezeHashReport` | Produce a non-promotional hash report for curation. |
| `ValidateGoldenAdmission` | Enforce the complete frozen 500-case admission contract. |
| `Evaluate` | Bind and score one exact, ordered, single-run corpus observation. |
| `CriticalSlices` | Return the immutable v1 critical-slice names. |
| `DecodeRegressionCorpus` / `DecodeRegressionAudit` | Strictly decode the independent regression admission pair. |
| `RegressionCorpusContentSHA256` / `RegressionAuditContentSHA256` | Compute the self-field-cleared two-way admission bindings. |
| `ValidateRegressionAdmission` | Require exact counts, passed semantic audit, and corpus/audit hashes without accepting human authority. |
| `DecodeRegressionObservationSet` / `EvaluateRegression` | Bind and score one exact ordered regression capture through the shared scorer. |

## Layout

```text
memoryeval/
├── README.md
├── DESIGN.md
├── types.go
├── load.go
├── validate.go
├── evaluate.go
├── regression_validate.go
├── regression_evaluate.go
├── evaluate_test.go
└── fixtures_test.go
```

See [`DESIGN.md`](./DESIGN.md) and
[`../../../docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).

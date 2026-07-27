# memoryeval

`memoryeval` is the offline, no-network contract for Neo Chat Memory benchmark
artifacts. It validates synthetic Golden corpora and captured observations,
binds them to immutable hashes, and produces deterministic quality, latency,
token, cost, and authority-safety results.

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
```

The report path is exclusive and is never overwritten. A failing gate still
publishes its complete report before the command returns a non-zero status.

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

## Layout

```text
memoryeval/
├── README.md
├── DESIGN.md
├── types.go
├── load.go
├── validate.go
├── evaluate.go
├── evaluate_test.go
└── fixtures_test.go
```

See [`DESIGN.md`](./DESIGN.md) and
[`../../../docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).

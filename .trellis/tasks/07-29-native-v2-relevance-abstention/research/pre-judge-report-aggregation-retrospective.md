# Pre-judge report aggregation retrospective

## Bug analysis: valid fail-closed runtime state destroyed the evidence bundle

### 1. Root cause category

- **Primary: B — Cross-layer contract.** The hybrid reader correctly returned
  `no_memory` when retrieval/admission became unavailable before configured-
  judge egress, but the schema-v10 capture reporter interpreted the same state
  as corrupt capture data.
- **Secondary: E — Implicit assumption.** Reusing the schema-v4/v5 report
  builder also reused its historical assumption that every candidate-bearing
  case must have `AdmissionReady=true`. That assumption was valid for those
  immutable report schemas but not for schema v10's live BGE failure surface.
- **Secondary: D — Test coverage gap.** Fake protocol covered successful and
  judge-failure paths, but no schema-v10 report test supplied a non-empty
  candidate set with a strictly empty pre-judge retrieval failure.

### 2. Why the earlier fix did not prevent recurrence

1. Schema v9 fixed the immediate Tool-route diagnostic symptom by adding a
   route-specific `retrievalIncompleteCaseCount` contract. It did not establish
   a reusable stage-state rule for later report schemas.
2. Schema v10 intentionally reused the strict cloud-judge evaluator and report
   shape to avoid a copied scorer, but the reuse boundary included historical
   completeness admission instead of only shared evaluation/cost logic.
3. Offline fake providers did not reproduce the real 750 ms BGE admission
   failure, so the reporter/runtime disagreement appeared only after paid work
   had finished and the isolated runtime was already being torn down.

### 3. Prevention mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Runtime contract | Schema v10 accepts a pre-judge failure only with false admission/rerank/judge readiness, zero judge token bound, and empty Provider-sent/Final/Injected/prompt-token surfaces. | Done |
| P0 | Historical compatibility | Keep `BuildCloudJudgeDevelopmentReport` strict; enable the exception only through the schema-v10 wrapper. | Done |
| P0 | Regression test | Assert schema v10 aggregates the failure, decrements actual judge requests, and schema v4/v5 reject the same trace. | Done |
| P0 | Code-spec | Record the validation matrix, Good/Base/Bad case, and required assertions in both Memory benchmark and hybrid specs. | Done |
| P1 | Future schema review | For every new capture schema, enumerate legal states at `Prepare -> Admission -> Provider -> Final` before reusing an older report builder. | Done in specs/workflow |

### 4. Systematic expansion

- **Similar seams reviewed:** scalar calibration remains intentionally strict;
  schema-v7 Tool reports remain intentionally strict; schema-v9 already has an
  explicit retrieval-incomplete contract; schema-v10 is the only cloud-judge-
  shaped report that needs the bounded pre-judge exception.
- **Design improvement:** reporter reuse must separate immutable evaluation
  semantics from schema-specific trace completeness. A new schema may reuse
  scoring, but it must opt into its own legal stage-state matrix.
- **Process improvement:** live-run readiness must include one synthetic case
  for each fail-closed stage, not only Provider failure and invalid output.
- **Operational improvement:** a valid metric failure must still publish its
  aggregate evidence. Only authority drift, impossible state, or publication
  failure may remove the bundle.

### 5. Knowledge capture

- [x] Updated `.trellis/spec/backend/memory-v2-benchmark.md`.
- [x] Updated `.trellis/spec/backend/memory-v2-hybrid-shadow.md`.
- [x] Updated capture `README.md`, `DESIGN.md`, and the operator workflow.
- [x] Added focused regression coverage and preserved historical behavior.
- [x] Confirmed this repository has no `src/templates/markdown/spec` mirror to
  synchronize.

# memoryeval Design

## Goals

- Establish an executable, versioned Memory benchmark contract before any
  Memory v2 runtime, schema, worker, or Provider integration is added.
- Preserve the proven RAG promotion evidence shape: strict JSON, canonical
  freeze hash, Development/Validation/Holdout separation, raw-artifact hashes,
  one precommitted Holdout run, complete ordered observations, and immutable
  output.
- Give retrieval quality, current-fact selection, false injection, latency,
  token cost, Provider cost, and authority safety one deterministic scorer.
- Keep committed draft material synthetic, non-sensitive, and visibly
  ineligible for promotion.
- Reuse one scorer for formal and regression evidence while making it
  impossible to admit machine review as a formal Golden.

## Non-goals

- Capturing observations from the live API, database, Provider, or Hindsight.
- Creating the human-reviewed 500-case corpus or fabricating reviewer records.
- Activating, migrating, or changing the v1 Memory reader.
- Deciding future confidence thresholds for Memory extraction or L2/L3.

## Architecture

```text
synthetic fixture manifest (external, SHA-256 bound)
        |
        v
Golden JSON --strict decode--> structural validation --frozen admission--+
        |                                                              |
        +--canonical content hash                                       v
                                                      deterministic evaluator
observations JSON --strict decode--> ordered/bound single Holdout -------+
                                                                     |
                                                                     v
                                               exclusive JSON gate report
```

The regression path joins only below admission:

```text
regression corpus + passed deterministic audit --separate admission--+
regression observations --no Holdout UUID, exact visible order--------+
                                                                    |
                                                                    v
                                                    shared metric scorer
                                                                    |
                                                                    v
                                      explicit regression-only report
```

`types.go` defines the versioned wire types. `load.go` owns the bounded strict
decoder, case semantics, lifecycle validation, and canonical hash.
`evaluate.go` owns artifact binding, aggregation, per-slice gates, and stable
failure ordering. `cmd/memory-eval` owns filesystem input/output only.

## Decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Separate `memoryeval` from `rageval` | Memory has temporal, scope, persistence, and Provider-egress semantics that RAG evidence does not. | The evidence lifecycle is reused without coupling metric evolution. |
| Golden contains opaque Memory IDs, fixture aliases, and exclusions rather than Memory content | Reports and source control must not become another private Memory store. | A separately secured synthetic fixture manifest is required and hash-bound. |
| `promotionEligible` is always `false` in Golden and freeze-hash artifacts | A corpus or hash is not a passing result or reader authority. | Only a complete report can pass, and even that requires a separate operator promotion decision. |
| v1 criteria are exact constants | Silent threshold loosening would make reports incomparable. | Any threshold change requires a new schema/evaluator version. |
| Exactly 500 cases and `300/100/100` splits | Matches the frozen product decision and prevents undersized evidence. | Draft templates and unit fixtures cannot be admitted. |
| At least 50 cases in every critical slice, including `30/10/10` Development/Validation/Holdout coverage | Prevents aggregate quality or a tuneable-only slice from hiding Chinese, temporal, deletion, secret, or isolation failures. | Cases may carry multiple compatible slice labels. |
| Final and injected IDs must be subsets of earlier ranking stages | Observation producers must expose the actual selection chain. | Direct readers must still materialize the equivalent candidate/final stages. |
| Safety is derived from Golden exclusions and observed ID surfaces | Self-reported `leaked=false` flags are not evidence. | Producers must list persisted and Provider-sent Memory IDs. |
| Reports use exclusive hard-link publication | A failed or consumed Holdout artifact must not be overwritten. | Operators choose a new path for every attempt. |
| Regression types are not fields on `GoldenSet` | Machine review must not become a lifecycle value that formal admission might accidentally accept. | Strict decoders reject cross-lane artifacts before scoring. |
| Corpus and audit use a two-way hash binding | A structurally valid corpus without its passed semantic audit is not admitted. | The corpus content hash clears only its own and the audit-hash field; raw corpus bytes still bind the final audit hash. |
| Regression has no Holdout UUID or ordinal | Every machine-reviewed case is visible and tuneable. | The `holdout` split is diagnostic stratification only and reports cannot claim one-shot evidence. |
| Both lanes call `scoreEvaluation` | Metric fixes and safety gates must apply identically. | Only admission, provenance, and report authority differ. |

## Metric Semantics

- Candidate Recall@20 and Final Recall@5 are micro-averaged over expected
  relevant Memory IDs.
- Current-fact accuracy is case-level: all expected current IDs must reach the
  injected set and no `superseded` ID may reach it.
- False injection is case-level across the full corpus: any injected ID not in
  that case's relevant allowlist makes the case false.
- nDCG@5 and MRR@5 are diagnostics for later reranker comparison; v1 does not
  invent an absolute reranker gate before a baseline exists.
- P95/P99 use nearest-rank percentiles. A result at or beyond the 2-second
  boundary must record `hardCutoffApplied=true`; any result beyond it fails.
- Provider cost uses same-unit integer microunits and is evaluated as Memory
  cost divided by the corresponding chat cost.
- Cross-user/out-of-scope, deleted, secret, untrusted-source, and unauthorized
  Provider-egress case counts are zero-tolerance gates.

## Threat Model and Controls

| Threat | Control |
| --- | --- |
| Real chat or sensitive Memory copied into Git/evaluation | Frozen `syntheticOnly=true`, `containsRealUserData=false`, and `containsSensitiveData=false` policy; Golden stores opaque IDs only. |
| Duplicate JSON key shadows a security field | Recursive duplicate-key rejection before typed decoding. |
| Unknown schema fields are silently ignored | `DisallowUnknownFields`, one JSON value, and a 64 MiB hard limit. |
| Corpus is edited after review | Canonical frozen-content SHA-256 plus exact raw-file SHA-256 in the report. |
| Fixture or profile drifts | Fixture-manifest and configuration hashes are mandatory. |
| Holdout is tuned or replayed | Precommitted UUID, ordinal exactly one, ordered complete observations, and exclusive output. |
| Machine regression is presented as formal evidence | Separate schema, decoder, admission, CLI flags, provenance, and `promotionEligible=false`; human attestations are rejected. |
| A producer hides leakage behind booleans | The evaluator derives leakage from exclusion reasons and observed ID surfaces. |
| Evaluation accidentally changes production | No database, network, Provider, `usermemory`, feature-flag, or migration dependency. |

## Known Limitations

- PR1 defines the artifact and scorer contract only. Reader-specific capture
  adapters arrive with their owning runtime phases.
- nDCG/MRR are visible but cannot prove reranker benefit until the same frozen
  corpus has both baseline and candidate reports.
- The 30-day production cost circuit needs runtime telemetry in addition to
  the benchmark's same-window synthetic/provider cost ratio.
- Human review quality remains an operational process; the validator proves
  attestations and timing, not reviewer judgment.
- The deterministic regression audit detects the known v1 shortcut and slice
  failures, but it is still machine judgment and cannot certify promotion.

## Change History

- 2026-07-28: Added the v1 offline benchmark schema, validator, evaluator,
  immutable report command, and contract tests. No runtime behavior changed.
- 2026-07-29: Added separate machine-reviewed regression corpus/audit/
  observation/report schemas and evaluation using the shared scorer. Formal
  Golden admission remains unchanged.

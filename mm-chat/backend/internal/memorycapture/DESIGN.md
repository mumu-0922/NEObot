# Native Memory regression capture design

## Goals

- Measure the code and SQL used by the Server-mode Memory readers rather than
  an evaluator-side ranking copy.
- Preserve all candidate, final, counterfactual injection, persistence, and
  Provider-egress authority surfaces required by the shared scorer.
- Make every run reproducible from fixed synthetic inputs, immutable profile
  hashes, a versioned cost basis, and an isolated PostgreSQL runtime.
- Fail closed before live Provider work when inputs, cost, output, database,
  authorization, or credential authority is invalid.

## Non-goals

- Human Golden review, formal Holdout execution, or reader promotion.
- Extraction/writer quality evaluation.
- Production chat, Memory, Provider vault, prompt, Usage, or flag mutation.
- Presenting deterministic fake vectors/reranking as reader-quality evidence.

## Data flow

```text
protected synthetic artifacts
  -> strict replay/hash admission
  -> deterministic alias/UUID map
  -> privileged seed in fresh marked PostgreSQL 17
  -> fixed BGE projection population
  -> go_api_runtime production v1 reader
  -> reset v1 last-used side effect
  -> go_api_runtime production hybrid reader
       -> repository decorator captures RRF Top 20/final Top 5
       -> Provider decorator captures exact rerank document IDs
  -> strict ordered observations
  -> shared memoryeval scorer
  -> plaintext/credential leak scan
  -> exclusive private bundle; run-manifest linked last
```

The privileged seed connection is never used by query-time readers. Runtime
capture must report `current_user=go_api_runtime` and must find the exact
run-bound marker in a database whose name starts with
`mm_chat_memory_regression_`.

## Decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Isolated PostgreSQL plus production decorators | Live chat would pollute state; an offline reimplementation would measure a copy. | The runner is operationally heavier but exercises deployed ranking and authorization. |
| Separate `fake_protocol` profile ID | Deterministic fake vectors and reranking prove protocol only. | Fake reports can never be mistaken for `native_v2_hybrid` reader quality. |
| Counterfactual `injectedMemoryIds` | The shared current-fact/false-injection scorer needs an injection surface. | Final IDs are mirrored offline only; no prompt is changed. |
| Explicit same-unit cost basis | Zero or invented Provider cost could create a false pass. | Baseline Memory cost must be zero, candidate Memory cost positive, and both chat denominators identical. |
| Run manifest as final link | A directory-level multi-file transaction is unavailable. | Any publication error removes links created by that call; the manifest is the completion marker. |
| Deterministic audit time floor | The byte-replayed regression generator uses a fixed audit timestamp. | Observation `capturedAt` is never earlier than that admission timestamp; wall-clock start/end remain in the run manifest. |

## Trust boundaries and threats

### Live database contamination

Both DSNs are parsed before connection. They must name the same database with
the ephemeral prefix; the runtime DSN must set `role=go_api_runtime`. Seed and
runtime SQL independently re-check the database name, migration head, current
role, empty state, and run marker.

### Provider credential leakage

Live authorization is exact and run-bound. The key is accepted only from a
regular non-symlink mode-`0600` file, never argv or an environment value. The
Compose live runner receives it as a read-only bind mount. Bytes are cleared
after use, retained artifacts/logs/Docker metadata are scanned, and wrapper
teardown removes the temporary directory on success, error, `SIGINT`,
`SIGTERM`, or `SIGHUP`.

### Fixture plaintext leakage

Only opaque case/Memory IDs, hashes, counts, timings, costs, and bounded status
codes are retained. Exact queries and canonical Memory bodies from every
fixture state are scanned against observations, reports, the run manifest,
runner output, and Docker metadata before retention.

### Stale or unauthorized ranking output

Production SQL reauthorizes all lanes. Decorators fail closed on overlapping
cases, unknown IDs, duplicate IDs, mismatched assistant messages, or Provider
document cardinality drift. The strict observation decoder rechecks stage
subsets and exact corpus order.

## Known limitations

- A live 500-case capture consumes real SiliconFlow quota and can take many
  minutes under per-case hard cutoffs.
- The machine-visible regression split is never formal Holdout evidence and
  every result remains `promotionEligible=false`.
- Fake protocol relevance and latency metrics are intentionally meaningless;
  only lifecycle and authority invariants are evaluated.

## Change history

- **2026-07-29**: Initial production-reader capture, PostgreSQL isolation,
  fake/live Provider separation, exclusive publication, and teardown protocol.

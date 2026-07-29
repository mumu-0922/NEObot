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
       -> migration-064 derives transient local admission similarity
       -> fixed BGE rerank and either strict cloud judge or main-model Tool route
          run concurrently under their versioned Development profile
       -> judge ordinals intersect BGE order, or one exact search_memory({})
          call releases unchanged BGE order, before the token selector
       -> Provider decorator captures exact candidate-document IDs
       -> calibration-only recorder retains request-local rerank scores
  -> full observations, aggregate Development grid, or frozen Validation
  -> shared memoryeval scorer and exact Provider-cost gate
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
| Explicit same-unit cost basis | A zero total candidate cost or invented Provider price could create a false pass. | Baseline Memory cost must be zero, total candidate Memory cost positive, exact component prices may be zero when the official fixed-model rate is free, and both chat denominators remain identical. |
| Run manifest as final link | A directory-level multi-file transaction is unavailable. | Any publication error removes links created by that call; the manifest is the completion marker. |
| Deterministic audit time floor | The byte-replayed regression generator uses a fixed audit timestamp. | Observation `capturedAt` is never earlier than that admission timestamp; wall-clock start/end remain in the run manifest. |
| Pre-hashed split-safe calibration plan | Thresholds cannot be guessed from the first content-free run or tuned on Validation. | Before Provider construction, the config hash binds Development, the 20,301-pair grid/objective, calibration policy, models, limits, and costs. |
| Aggregate-only score handling | Raw intent/admission/rerank distributions would retain sensitive per-case retrieval evidence. | Scores exist only in process memory; schema-v3 reports contain failure counts, best attempts, and cumulative relevant/unrelated-negative threshold counts, with no observation files. |
| Query-only intent gate | Scalar query-to-Memory and rerank scores cannot separate unrelated cases without destroying recall. | The fixed reranker compares the redacted query only with two version/hash-bound non-user bilingual anchors before any Memory-document egress; failure or low margin is `no_memory`. |
| Owner-authorized cloud candidate judge | The confirmed single-user Server-mode policy allows ordinary current-user candidates to reach the configured Provider, and query-only signals were infeasible. | Schema v4 sends only redacted query/candidate bodies with request-local ordinals, never IDs/scope/revision/scores; forbidden authority reasons still fail and false-injection gates are unchanged. |
| Concurrent BGE and judge stages | Serial calls would spend most of the existing two-second hard cutoff. | Both stages share one bounded context; either failure, late result, provenance drift, or malformed judge JSON yields `no_memory`. |
| Strict judge contract | Free-form output could inject instructions, IDs, or unverifiable ranking data. | The fixed prompt accepts candidates as untrusted data and requires exactly `schemaVersion` plus at most five unique in-range ordinals; empty means `no_memory`. |
| Explicit cloud-judge cost ceiling | Per-token judge quota cannot be inferred from the historical aggregate Memory cost. | Cost-basis v2 binds 300 requests, conservative input/output token ceilings, exact prices (including an official free rate), and maximum judge cost before Provider construction. |
| Versioned owner absolute budget | A paid stronger judge is guaranteed to fail the historical relative-cost criterion even when the owner explicitly does not select on expense. | Cost-basis/profile/report schema v3/v5 binds `owner_authorized_absolute_cap_v1`; ratio stays truthful and informational while exact absolute ceilings remain mandatory. Historical schema-v4 semantics do not change. |
| Main-model Tool route | Three candidate-aware hosted judge models failed unchanged quality/latency gates, and the owner already selected GPT/DeepSeek for chat. | Schema v6 sends only the redacted query plus the exact `search_memory` Tool to one bound current model; no candidate body reaches that route boundary. |
| Exact empty-object call | Missing arguments and `{}` have different decoding provenance even though both can look empty in Go. | The adapter requires a non-nil empty map, one non-empty call ID, one exact name, and no duplicate calls. |
| Speculative BGE overlap | A separate route round plus serial embedding/rerank would make the unchanged two-second gate harder to meet. | BGE work may overlap, but its candidates stay request-local and are discarded unless the exact route call succeeds. |
| Independent live credentials | BGE and the selected chat route are separate Provider authorities. | Cost-basis v4 and the wrapper reject the same file, hard links, or equal Key bytes and bind each exact target independently. |
| Candidate failure means `no_memory` | v1 remains the real prompt authority but is a separate benchmark profile. | Prepare/Record/Provider/cutoff failures never launder v1 or unscored RRF rows into v2 final/injected surfaces. |

## Trust boundaries and threats

### Live database contamination

Both DSNs are parsed before connection. They must name the same database with
the ephemeral prefix; the runtime DSN must set `role=go_api_runtime`. Seed and
runtime SQL independently re-check the database name, migration head, current
role, empty state, and run marker.

### Provider credential leakage

Live authorization is exact and run-bound. Each key is accepted only from a
regular non-symlink mode-`0600` file, never argv or an environment value. The
Compose live runner receives it as a read-only bind mount. Tool-route mode uses
one SiliconFlow BGE Key and one independently approved GPT/DeepSeek Key; the
runner rejects the same file, hard links, and equal byte content. Both byte
buffers are cleared after use, retained artifacts/logs/Docker metadata are
scanned, and wrapper teardown removes the temporary directory on success,
error, `SIGINT`, `SIGTERM`, or `SIGHUP`.

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

Migration `064` reauthorizes the complete pending RRF surface before Memory
document egress and derives only the maximum cosine signal. Missing, stale,
low-confidence, invalid, redacted, failed, or late Provider evidence produces
`no_memory`. Provider output returned after its context deadline is not marked
rerank-ready by the recorder and cannot become calibration authority.

Under the exact schema-v4 owner policy, ordinary candidates excluded only as
`irrelevant` may reach the cloud judge and BGE reranker. Cross-user,
out-of-scope, deleted, secret, superseded, Sensitive-disabled, and
untrusted-source candidates remain zero-tolerance violations. The shared
evaluator still scores final false injection independently, so egress
authorization never authorizes prompt injection.

The judge prompt is shared by production and capture adapters and is bound by
version, SHA-256, model ID, and decoding profile. Its Provider payload contains
only secret-redacted query text plus candidate bodies labelled with contiguous
request-local ordinals. It contains no Memory ID, revision, scope, authority
field, or retrieval score. Exact-key/duplicate-key/size/cardinality/range
validation and post-call provenance checks fail closed.

The main-model Tool definition is shared by capture and future product
integration through `internal/memoryroute`. Its Provider request contains only
the secret-redacted current query and the exact no-argument `search_memory`
definition. The adapter accepts zero calls or exactly one non-empty-ID call
whose decoded arguments are a non-nil empty object. Any other choice/call/
argument shape fails closed. The returned boolean carries exact model,
contract-version, and contract-hash provenance; it never carries free-form
model output or candidate authority.

## Known limitations

- Live Development/Validation capture consumes real SiliconFlow quota and can
  take many minutes under per-case hard cutoffs. Live full-regression mode is
  forbidden; the phases use separate 300/100-case runs.
- The machine-visible regression split is never formal Holdout evidence and
  every result remains `promotionEligible=false`.
- The first live fixed scalar Development grid produced `20,301/0` feasible
  pairs while passing the cost gate. Its schema-v1 feasible-only frontier is
  insufficient to select a dynamic policy. The completed schema-v2 aggregate
  diagnostic run ruled out scalar, max-score, and candidate-margin policies;
  the completed schema-v3 query-only intent-margin run also found `0/201`
  feasible thresholds.
- The first schema-v4 `Qwen/Qwen3-8B` Development run failed relevance and
  latency gates. The schema-v5 `deepseek-ai/DeepSeek-V4-Flash` owner absolute-
  budget follow-up also failed: 164/195 judge requests hit the unchanged hard
  cutoff. The next named Development hypothesis was the 3B-active
  `Qwen/Qwen3.6-35B-A3B`. Until Development passes, the cloud policy/model/
  prompt/decoding profile is not frozen for Validation,
  `HybridShadowFrozenPolicy()` remains unavailable, and promotion stays
  disabled.
- Qwen3.6 subsequently failed with 40/195 cutoff events plus recall and false-
  injection failures. Qwen3.5-4B was cancelled without Provider construction
  or quota use when the owner chose the main-model Tool architecture.
- Schema-v6 Tool routing is implemented and proven through a 300-case
  PostgreSQL 17 fake-protocol replay, but no live GPT or DeepSeek Development
  result exists. The Development prompt is the current redacted query rather
  than the full product conversation replay, product same-model continuation is
  not wired, Validation remains blocked, and promotion remains disabled.
- Fake protocol relevance and latency metrics are intentionally meaningless;
  only lifecycle and authority invariants are evaluated.

## Change history

- **2026-07-29**: Initial production-reader capture, PostgreSQL isolation,
  fake/live Provider separation, exclusive publication, and teardown protocol.
- **2026-07-29**: Split-safe two-stage relevance calibration, migration-064
  pre-rerank admission, request-local score abstention, aggregate-only
  evidence, and frozen-Validation denial until code freeze.
- **2026-07-29**: Fixed scalar Development result recorded as infeasible;
  schema-v2 aggregate failure/attempt/threshold diagnostics added without
  retaining case identity, plaintext, or raw scores.
- **2026-07-29**: Schema-v2 diagnostics ruled out scalar/max-score/candidate-
  margin policies; schema-v3 query-only bilingual intent-margin calibration
  added behind Development-only/default-off authority.
- **2026-07-29**: Schema-v3 Development found no feasible intent threshold;
  no policy was frozen and Validation stayed denied.
- **2026-07-29**: Owner-authorized schema-v4 cloud candidate judge added with
  strict ordinal output, BGE concurrency, policy-aware egress scoring,
  cost-basis v2, Development-only aggregate evidence, and fail-closed runtime
  task-model resolution. Its first Qwen3-8B Development run failed relevance
  and latency gates and selected no policy.
- **2026-07-29**: Schema-v5 owner absolute-cap cost policy added for the
  precommitted paid `DeepSeek-V4-Flash` follow-up, preserving every non-cost
  gate and historical schema-v4 cost semantics.
- **2026-07-29**: DeepSeek-V4-Flash Development failed the hard-cutoff/recall
  gates; Qwen3.6-35B-A3B was named as the next fresh Development hypothesis.
- **2026-07-29**: Qwen3.6-35B-A3B also failed cutoff/recall/false-injection
  gates; Qwen3.5-4B was cancelled before execution in favor of an architecture
  pivot.
- **2026-07-29**: Schema-v6 main-model `search_memory` Tool routing added with
  exact OpenAI/OpenAI-compatible decoding, Provider/model/Base-URL hash
  authority, independent dual credentials, cost-basis v4, aggregate-only
  evidence, and a successful 300-case fake-protocol lifecycle replay. No live
  route-model quality result or production activation was created.

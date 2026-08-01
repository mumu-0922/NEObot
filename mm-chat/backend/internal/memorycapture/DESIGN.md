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
       -> fixed BGE rerank and either strict cloud/configured judge or
          historical main-model Tool route run concurrently under its
          versioned Development profile
       -> judge ordinals intersect BGE order, or one exact search_memory({})
          call releases unchanged BGE order, before the token selector
       -> Provider decorator captures exact candidate-document IDs
       -> calibration-only recorder retains request-local rerank scores
       -> route completion closes before Recorder.Finish; writes carry the
          originating capture generation
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
| Explicit known-version regression root | V2/v3/v4 evidence must remain immutable while each repaired corpus needs the same production capture path. | The wrapper defaults to v2 for compatibility, `--regression-root` may select exact v3/v4/v5, generator replay rejects unknown/mixed pools, and raw input hashes force distinct configuration SHA-256 values for all versions. |
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
| Main-model Tool route | Three candidate-aware hosted judge models failed unchanged quality/latency gates, and the owner already selected GPT/DeepSeek for chat. | Schema v6 preserves the failed preflight; schema v7 sends the current synthetic query/message plus canonical `search_memory` through one real first `ToolRoundProvider` request. |
| Exact empty-object call | Missing arguments and `{}` have different decoding provenance even though both can look empty in Go. | The adapter requires a non-nil empty map, one non-empty call ID, one exact name, and no duplicate calls. |
| Speculative BGE overlap | A separate route round plus serial embedding/rerank would make the unchanged two-second gate harder to meet. | BGE work may overlap, but its candidates stay request-local and are discarded unless the exact route call succeeds. |
| Route lifecycle is generation-bound | Admission can fail closed before a concurrent route returns; an identity-only Recorder can then misclassify or accept a late write. | One replayable route completion closes on every exit, delegated calls publish through a buffered context-selected result, and old-generation writes fail closed. |
| Independent live credentials | BGE and the selected chat route are separate Provider authorities. | Cost-basis v5 and the wrapper reject the same file, hard links, or equal Key bytes and bind each exact target independently. |
| Candidate-first configured judge | Candidate-blind routes cannot discover implicit personalization, while candidate recall itself is not prompt injection. | Schema v10 recalls and reauthorizes first, then reuses the strict ordinal judge with the exact configured GPT/DeepSeek Provider; only judge/BGE intersection may become a counterfactual final set. |
| Stage-local failure aggregation | A runtime may correctly fail closed before configured-judge egress even though candidate recall was non-empty; treating that as whole-bundle corruption destroys valid failed-gate evidence. | Schema v10 aggregates the normalized failure only when every judge/request/final/token surface proves `no_memory`; schema v4/v5 preserve their historical strict rejection. |
| Fixed global Memory Judge | Answer-model-specific judges failed schema-v10 gates and the owner rejected local inference. | Schema v11 fixes `SERVER_DEFAULT/openai_compatible/gpt-5.6-luna`, criteria v2, cost-basis v7, and a 3000-ms complete-flow cutoff; failure releases no v2 Memory and never falls back to unjudged candidates. |
| Accuracy-first execution | Schema-v11 short cutoffs converted Provider latency into missing decisions, and intra-case concurrency overloaded the shared route without improving correctness. | Schema v12 fixes query-embedding/admission/rerank/judge/Record order, global Provider concurrency one, no application/HTTP elapsed timeout, diagnostic-only latency, a one-second inter-case cooldown, and one bounded transient retry. |
| Attempt-derived cost | Retrying without aggregate authority would hide quota and token amplification. | Cost-basis v8 pre-authorizes at most 600 Judge attempts/76800 output tokens; report validation reconciles attempt counts, total/retry Judge input bounds, and `attempts * 128` output authority. |
| Manual stage isolation | Development evidence is not Validation or production authority. | Every schema-v11/v12 Development run stops for owner review; Validation and production activation require separate authorization. |
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

Configured candidate-judge mode uses the same independent-credential boundary
with a distinct cost-basis-v6 Provider authority and mount. Candidate bodies
may reach only that exact owner-authorized configured Provider after current
authority and secret redaction; neither credential value enters argv,
environment, retained output, or Docker metadata.

Fixed Memory Judge mode further requires the exact schema-v11 Luna tuple and
cost-basis v7. The wrapper overwrites and removes both temporary Key copies on
success, error, and signal; the runner receives read-only files and no Vault
decryption authority.

Accuracy-first mode retains the same two-credential boundary under schema v12
and cost-basis v8. Its BGE and Luna HTTP clients reject redirects and
environment proxies, require TLS 1.2 or newer, and impose no elapsed timeout.
Manual cancellation is the only interruption authority. Only 408/429/5xx and
retryable transport/read interruptions may retry once; deterministic protocol
failures do not gain extra egress.

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

The main-model Tool definition/hash/validation is owned by `internal/chat`.
`internal/memoryroute` delegates to that authority for capture only. Its schema-
v7 Provider request contains the current synthetic query/message and exact
no-argument `search_memory` definition, with no candidate body. The adapter
accepts zero calls or exactly one non-empty-ID call whose decoded arguments are
a non-nil empty object. Any other event/call/argument shape fails closed. The
returned boolean carries exact model, contract-version, and contract-hash
provenance; it never carries free-form output or candidate authority.

The decorator, not the delegated router goroutine, owns Recorder publication.
It selects one buffered result against route-context termination and returns a
bounded context category when the delegate ignores cancellation. Route input
returns a per-case generation token; only that token can record the matching
result or failure, even when a later case reuses the same assistant identity.

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
- Schema-v6 Tool routing has a 300-case PostgreSQL 17 fake-protocol replay and
  three immutable live outcomes: GPT failed, the first DeepSeek run is protocol-
  invalid, and corrected DeepSeek Flash failed. Its `PlanTools` preflight is
  rejected.
- Schema-v7 first-ToolRound routing has offline protocol/report/lifecycle,
  migration-065 PostgreSQL evidence, and valid failed GPT and DeepSeek Flash
  Development results: only `28/300` and `33/300` routes completed, and both
  failed unchanged quality, slice, cutoff, and latency gates. Capture still
  uses one synthetic current query rather than full conversation replay and
  does not execute the product answer continuation. Validation remains blocked
  and promotion remains disabled.
- Schema v8 introduced route-failure diagnostics with a fixed typed taxonomy,
  but two paid attempts published no artifact; the second stopped at bounded
  `admission_state`. Schema v9 preserves the same v7 request shape and route
  taxonomy while separating fail-closed retrieval incompleteness into aggregate
  counts. Incomplete retrieval must have empty Final/Injected/token surfaces.
  Upstream error text, response bodies, queries, Tool payloads, Memory content,
  scores, and case identity do not survive. The diagnostic lane cannot select a
  policy even when unchanged metrics pass.
- The first schema-v9 live artifact recorded `31` context deadlines, `83`
  invalid Tool Calls, and `174` unclassified failures. Offline tracing proved
  one concrete producer: admission-unavailable paths did not await their
  already-started route. The lifecycle/generation repair prevents recurrence
  but cannot relabel the immutable identity-free artifact.
- Schema v9 ends candidate-blind routing. Schema v10 is a new Development-only
  candidate-first hypothesis using the exact configured GPT or DeepSeek model
  through the shared strict judge adapter. Offline/fake profile, report,
  manifest, cost, authorization, CLI, Compose, and cleanup gates exist. The
  authorized GPT profile completed no strict judge decision and failed recall/
  latency. The independent DeepSeek profile completed `157/195` decisions but
  reached only `0.558974/0.581818` Final Recall@5/current-fact and also failed
  latency. Both retained zero false injection and zero authority/privacy leaks,
  but neither selected a policy. Validation, runtime installation, and
  promotion remain unavailable.
- Schema v11 is the separately versioned fixed-Luna successor. It binds reader
  v9, profile/report v11, criteria v2 (`1500/2500/3000 ms`), cost-basis v7,
  exact Provider/Base-URL/model authority, and the unchanged strict ordinal/
  BGE intersection contract. Timeout, invalid output, Provider failure, and
  protocol drift return empty v2 Memory while v1 chat continues. Development
  cannot chain into Validation or production.
- The retained schema-v11 Development run failed with only `41` Luna attempts,
  `22` complete rerank-plus-judge decisions, `154` admission-unavailable cases,
  and `19` `HARD_CUTOFF` complete stages. Its report bytes and v2 criteria stay
  immutable.
- Schema v12 is the accuracy-first successor: reader v10, profile/report v12,
  criteria v3, cost-basis v8, serial BGE-before-Luna execution, global Provider
  concurrency one, no elapsed cutoff, and diagnostic-only latency. Fake mode
  records 299 virtual cooldowns with zero elapsed time; live mode performs 299
  wall-clock one-second cooldowns. All attempts, retries, per-stage timings,
  cooldowns, and Judge token upper bounds are aggregate-only.
- The first v12 v2 run and the separately authorized repaired-v3 run are both
  immutable failed Development evidence. V3 improved false injection from
  `29/300` to `10/300` and improved Final Recall@5/current-fact accuracy to
  `0.984615/0.981818`, but false injection `0.033333` and `stable_fact`
  accuracy `0.933333` still failed. Neither result selects a policy or opens
  Validation, production, promotion, or an automatic paid rerun.
- The separately authorized v4 run reached `1.0` Candidate Recall@20, Final
  Recall@5, and current-fact accuracy with zero safety/authority leaks, but one
  of 30 weather-board `unrelated_negative` cases still exceeded the slice
  false-injection gate. V4 evidence is immutable and aggregate-only; its exact
  false-positive case is not recoverable.
- V5 replaced the weather/facilities family with a universally separated
  physical mug-location hard negative. Its run still produced one of 30
  unrelated false injections and also recorded `17` `CANDIDATE_JUDGE_FAILED`
  cases (`217` attempts, `22` retries), reducing Final Recall@5/current-fact to
  `0.907692/0.909091`. Aggregate-only evidence cannot assign that positive
  decline to corpus semantics. V5 is immutable failed evidence and grants no
  rerun, Validation, production, or promotion authority.
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
  evidence, and a successful 300-case fake-protocol lifecycle replay.
- **2026-07-30**: Retained failed schema-v6 GPT/DeepSeek evidence, rejected the
  separate `PlanTools` preflight, and added schema-v7 first-`ToolRoundProvider`
  capture with canonical chat authority, cost-basis v5, and a distinct artifact.
- **2026-07-30**: Recorded the first schema-v7 GPT Development failure without
  changing gates or rollout authority; transient credentials and isolated
  runtime state were destroyed after aggregate publication.
- **2026-07-30**: Recorded the independent schema-v7 DeepSeek Flash failure
  under the same unchanged gates and cleanup boundary; no profile was frozen.
- **2026-07-30**: Added schema-v8 Development-only failure diagnostics after
  proving that v7 collapsed HTTP, transport, SSE, Tool, provenance, and capture
  failures into one code. Two paid attempts produced no artifact; the second
  bounded the rejection to `admission_state`.
- **2026-07-30**: Added schema-v9 route-only diagnostic completeness so
  fail-closed admission/rerank results are aggregated separately without
  weakening cutoffs, retries, empty-final authority, or promotion denial.
- **2026-07-30**: Closed every started route before capture completion, bounded
  cancellation-ignoring delegates through a buffered context select, and
  generation-fenced Recorder route writes after the offline schema-v9 trace.
- **2026-07-31**: Stopped candidate-blind routing and added the schema-v10
  candidate-first configured GPT/DeepSeek judge Development lane with reader
  v8, adapter provenance, cost-basis v6, independent credentials, aggregate-
  only evidence, and no live or production activation.
- **2026-07-31**: Recorded separate failed-gate GPT and DeepSeek schema-v10 live
  profiles, retained zero false injection/authority leaks, and kept Validation
  blocked. Allowed strictly empty pre-judge retrieval failures to aggregate in
  schema v10 without changing schema-v4/v5 report semantics.
- **2026-07-31**: Added the schema-v11 fixed global Luna Development lane with
  criteria-v2 latency budgets, reader v9, cost-basis v7, exact two-credential
  isolation, fail-closed no-fallback semantics, and mandatory manual stops
  before Validation and production.
- **2026-07-31**: Preserved the failed schema-v11 bundle and added schema-v12
  accuracy-first Development with reader v10, criteria v3, cost-basis v8,
  global serial Provider execution, no elapsed deadlines, diagnostic-only
  latency, bounded transient retry, virtual/live cooldown clocks, reconciled
  attempt/token telemetry, and an unconditional stop before Validation.
- **2026-07-31**: Recorded the separately authorized repaired-v3 schema-v12
  Development failure: all 300 cases completed, false injection fell to
  `10/300` but remained above criterion, `stable_fact` also failed, and all
  credentials/runtime objects were destroyed without Validation or promotion.
- **2026-08-01**: Admitted the separately authored v4 corpus through one
  schema-v12 fake-protocol Development lifecycle. All 300 cases completed,
  publication/leak/teardown checks passed, and the non-quality fake judge
  failed the false-injection gate without credentials, Provider egress,
  Validation, or promotion.
- **2026-08-01**: Recorded the separately authorized v4 schema-v12 live
  Development failure: the three recall/accuracy metrics were `1.0` but one of
  30 hard negatives exceeded the unchanged per-slice false-injection gate.
- **2026-08-01**: Added the immutable v5 universal-negative corpus identity and
  recorded its separately authorized failed live bundle. One hard negative
  still injected, while 17 bounded Judge failures prevented attribution of the
  positive-quality decline; teardown completed and Validation stayed blocked.

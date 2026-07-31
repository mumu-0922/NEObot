# Native v2 relevance abstention

## Goal

Add a versioned, split-calibrated abstention policy to the native v2 hybrid
Memory reader. Recall current-authorized candidates before admission, then let
one exact, globally fixed Memory Judge select directly useful candidate
ordinals through the strict candidate-judge contract before any Memory enters
the answer prompt. Preserve BGE-M3 retrieval/rerank and the current v1 prompt
authority until independent evidence passes. Historical configured-main-model
profiles remain immutable failed evidence; the current successor is the exact
`sub.mumubuku.top` / `gpt-5.6-luna` cloud profile.

## What I already know

- The first live 500-case run completed successfully as an execution but failed
  quality gates.
- v2 achieved 100% semantic recall/current-fact/ranking metrics, but produced
  50 false injections and 50 unauthorized Provider-egress events.
- All 50 failures were the 50 `unrelated_negative` cases; cross-user, deleted,
  secret, and untrusted-source leak counts remained zero.
- v2 is still a non-promotional candidate. The active v1 reader and production
  prompt/Usage behavior were not changed.
- The current rerank code sorts by score and then discards the score. Final
  selection has Top-5/token gates but no relevance abstention.
- The retained run did not store raw scores, so guessing a threshold from that
  evidence would be invalid.

## Confirmed Product Policy

- Precision is preferred over optional Memory recall when confidence or
  Provider authority is uncertain.
- This is a single-owner Server-mode deployment. The owner explicitly
  authorizes current-user, current-scope, non-secret canonical Memory candidates
  to be processed by the configured cloud Provider even when the query later
  proves unrelated. Relevance is injection authority, not cloud-egress
  authority, under this opt-in profile.
- Cross-user, out-of-scope, deleted, secret, Sensitive-disabled, superseded, and
  untrusted-source Memory remain forbidden at every Provider boundary. API
  keys, passwords, and detected secrets remain redacted regardless of the
  owner's general cloud-processing authorization.
- Missing/invalid admission evidence, low score, redaction, timeout, stale
  authority, or Provider failure returns no hybrid Memory. It never falls back
  to unscored RRF or v1 candidate injection.
- Normal chat continues without hybrid Memory; the current v1 production
  prompt authority remains unchanged while this work is shadow-only.

## Development calibration outcome

- The first authorized 300-case Development calibration completed on
  2026-07-29 and evaluated all `20,301` fixed scalar pairs.
- `providerCostRatio=0.033084` passed the unchanged cost gate, but
  `feasiblePairCount=0`; therefore the fixed admission/final scalar policy is
  infeasible on Development and no policy was selected.
- Validation remains blocked. No threshold may be guessed, no evaluator gate
  may be weakened, and the visible machine Holdout remains unavailable for
  tuning.
- The v1 aggregate frontier records only feasible points. Because there were
  none and request-local traces were destroyed as designed, it cannot explain
  the recall/safety conflict or justify a dynamic policy.
- A schema-v2, aggregate-only diagnostic rerun is required with a fresh Key.
  It records failure-pair counts, best safety/recall attempts, and cumulative
  admission/max-rerank/top-two-margin curves without case identity, plaintext,
  or raw per-case scores.
- That schema-v2 rerun completed all `20,301` pairs with the same `0`
  feasible result. The first zero-egress admission threshold (`0.85`) retains
  only `20/165` relevant cases; max rerank reaches zero unrelated cases only at
  `1.00`, retaining `0/165`; and fail-closed top-two margin retains at most
  `30/165` because all `30` unrelated and `135` relevant cases have one
  reranked candidate.
- The schema-v3 query-only bilingual intent run completed all `201` fixed
  margin thresholds with `providerCostRatio=0.056284` passing but
  `intentFeasibleThresholdCount=0`; no policy was selected.
- The first zero-egress intent threshold (`0.04`) retained only `31/165`
  relevant current-fact cases (`18.79%`). The recall-first threshold (`-0.09`)
  reached full recall/current-fact accuracy but caused `26` false injections
  and `26` unauthorized Memory-document egress events.
- Query-only intent, scalar local admission, maximum rerank score, and
  candidate margin are therefore all infeasible on Development. Validation
  remains blocked. The owner has now approved the alternate trust boundary: a
  cloud candidate judge may inspect already-reauthorized normal candidates and
  must return an exact selected set or `no_memory` before prompt injection.
- The first schema-v4 cloud-judge Development run completed on 2026-07-29.
  Candidate Recall@20 remained `1.0`, all authority-leak counters remained
  zero, and `providerCostRatio=0.110916` passed, but Final Recall@5 was
  `0.758974`, current-fact accuracy was `0.751515`, and false injection was
  `14/300`. The shared provider stage failed closed at cutoff for `31/195`
  judge requests; p95/p99 latency was `1853/1855 ms` and failed the unchanged
  latency gate. No policy was frozen, Validation remains blocked, and the
  source Key was destroyed.
- The owner explicitly removed relative Provider expense as a selection
  criterion for this single-owner deployment. A paid follow-up must not mutate
  schema v4 or fake a passing cost ratio: it uses a new
  `owner_authorized_absolute_cap_v1` policy that truthfully reports the ratio
  while enforcing exact hash-bound request/token/price/absolute-cost ceilings.
- The next precommitted model hypothesis is
  `deepseek-ai/DeepSeek-V4-Flash`. Its prompt/schema/decoding, two-second
  cutoff, and every relevance, safety, latency, token, split, privacy, and
  promotion gate remain unchanged. Validation remains blocked until the new
  Development profile passes.
- That schema-v5 Development run completed but failed: `164/195` judge calls
  hit the hard cutoff, Final Recall@5/current-fact accuracy fell to
  `0.143590/0.145455`, and p95/p99 latency was `1856/1865 ms`. False injection
  and every authority/privacy leak count were zero. The Key/runtime were
  destroyed and Validation remains blocked.
- The next separately versioned Development model hypothesis is
  `Qwen/Qwen3.6-35B-A3B`, selected for its stronger current model class and
  3B-active MoE latency profile. It reuses no credential or cost basis and
  changes no prompt, cutoff, quality, safety, latency, token, privacy, split,
  or promotion gate.
- Qwen3.6 Development also failed: Final Recall@5/current-fact was
  `0.733333/0.733333`, false injection was `15/300`, p95/p99 was
  `1854/1856 ms`, and `40/195` judge requests hit the hard cutoff. The
  Key/runtime were destroyed; Validation remains blocked.
- The planned `Qwen/Qwen3.5-4B` run was cancelled before Provider construction
  or quota use. Its empty credential and unused private cost basis were
  destroyed. This is `cancelled_not_run_architecture_pivot`, not a fabricated
  model result.
- The owner selected a main-model Tool architecture: the configured GPT or
  DeepSeek model sees the exact `search_memory` definition but no Memory body,
  then the unchanged BGE-M3 path supplies a bounded result only after a valid
  Tool Call. Further hidden-Qwen model hopping is stopped.
- Passing all candidates to the answer model in one prompt is rejected because
  those candidates would already have entered the answer prompt before the
  relevance decision, silently weakening the unchanged false-injection gate.
- The schema-v6 GPT Development run completed only `41/300` route decisions;
  `250` cases reported `HARD_CUTOFF` and `9` reported
  `MEMORY_TOOL_ROUTE_FAILED`. Final Recall@5/current-fact accuracy was
  `0.087179/0.090909`, false injection was `2/300`, and p95/p99 was
  `2002/2003 ms`. No policy was frozen.
- The first DeepSeek Pro execution is retained as
  `protocol_mismatch_invalid_quality_evidence`, not as a model failure. The
  adapter sent generic `enable_thinking=false` to official
  `api.deepseek.com`, whose contract requires
  `thinking.type=disabled`.
- After correcting that protocol, DeepSeek Flash completed `77/300` route
  decisions and failed `223`; `221` carried `MEMORY_TOOL_ROUTE_FAILED` and `2`
  carried `HARD_CUTOFF`. Final Recall@5/current-fact accuracy was
  `0.256410/0.254545`, false injection was `3/300`, and p95/p99 was
  `1377/1808 ms`. Every authority/privacy leak count remained zero, but quality
  and latency gates failed and no policy was frozen.
- The implementation used an extra non-streaming `PlanTools` preflight before
  the normal answer request. It did not reuse the existing first chat Tool
  round. This preflight hypothesis is rejected; raising the cutoff or retrying
  it would only hide request amplification.
- The first schema-v7 `SERVER_DEFAULT/gpt-5.6-sol` Development run used the
  real first `ToolRoundProvider` request and completed only `28/300` route
  decisions, all of which called Memory. The remaining `272` failed closed:
  `266` reported `HARD_CUTOFF` and `6` reported
  `MEMORY_TOOL_ROUTE_FAILED`. Candidate Recall@20 remained `1.0`, but Final
  Recall@5/current-fact accuracy fell to `0.102564/0.109091`; false injection
  was `2/300`, and p95/p99 was `2002/2002 ms`. Every authority/privacy leak
  count remained zero. The profile failed unchanged quality, slice, cutoff,
  and latency gates; no policy was frozen.
- The independent schema-v7 `FOHWSU/deepseek-v4-flash` Development run
  completed only `33/300` route decisions, all of which called Memory. The
  remaining `267` failed closed: `4` reported `HARD_CUTOFF` and `263` reported
  `MEMORY_TOOL_ROUTE_FAILED`. Final Recall@5/current-fact accuracy was
  `0.128205/0.127273`, false injection was `2/300`, and p95/p99 was
  `1622/1860 ms`; the evaluator recorded one hard-cutoff violation. Every
  authority/privacy leak count remained zero. This profile also failed
  unchanged quality, unrelated-negative, cutoff, and latency gates; no policy
  was frozen.
- Local source tracing proved that schema-v7 collapsed HTTP, transport, SSE,
  context, Tool Call, provenance, and recorder failures into the same
  `MEMORY_TOOL_ROUTE_FAILED` count. The retained aggregate evidence cannot
  retroactively identify the `263` DeepSeek subtypes. A schema-v8,
  Development-only diagnostic lane now binds a fixed category taxonomy and
  emits aggregate subtype counts only; it has no policy-selection authority.
- Two separately authorized schema-v8 attempts consumed quota but published no
  artifacts. The first returned only the historical generic post-capture
  integrity error. After bounded integrity reasons were added, the second run
  `memory-regression-20260730t052917z-7b8c8bcf` returned
  `Memory Tool-route report admission_state`, proving at least one non-empty
  candidate case had incomplete admission without identifying whether BGE
  timeout, invalid response, or SQL admission caused it. Both isolated runtimes
  and all transient credentials/helpers were destroyed.
- Schema v9 is the route-only diagnostic successor. It preserves the route
  taxonomy and fixed Provider/cutoff/no-retry behavior, permits fail-closed
  admission/rerank incompleteness only when Final/Injected/tokens are empty,
  and records separate aggregate retrieval-incomplete counts. It still has no
  policy-selection, Validation, production, or Promotion authority.
- The owner-authorized schema-v9 run
  `memory-regression-20260730t094556z-0f4878dd` published a valid private
  report plus manifest and failed the unchanged gates. Only `12/300` routes
  completed, all calling Memory; `288` failed closed as `31`
  `CONTEXT_DEADLINE`, `83` `TOOL_CALL_INVALID`, and `174`
  `ROUTER_FAILURE_UNCLASSIFIED`. The independent retrieval aggregate recorded
  `174` `RELEVANCE_ADMISSION_UNAVAILABLE` cases. Final Recall@5/current-fact
  accuracy was `0.010256/0.012121`, false injection was zero, and p95/p99 was
  `2001/2002 ms`; every authority/privacy counter remained zero. This is valid
  diagnostic and failed-metric evidence only. No policy was selected, and
  Validation/Promotion remain blocked.
- Offline source tracing then proved a diagnostic lifecycle defect: when query
  embedding/admission became unavailable after the route started, the reader
  returned without awaiting the route, and capture synthesized an unclassified
  category from incomplete Recorder state. The route stage is now replayable
  and closed on all exits, delegated cancellation-ignoring routers cannot hold
  the reader, and Recorder writes are generation-bound. This does not rewrite
  the immutable identity-free v9 artifact or authorize another paid run.
- The owner then rejected candidate-blind routing as the next architecture.
  Candidate recall and prompt injection are now separate authorities: private
  recall may run first, but only a candidate-aware admission result may release
  Memory to the answer model. The schema-v6/v7/v9 Tool-route line is historical
  failed evidence and receives no further diagnostic work.
- The next Development-only hypothesis reuses the strict candidate ordinal
  judge with the exact configured GPT or DeepSeek Provider. This is distinct
  from the failed SiliconFlow candidate judges and from the failed configured
  main-model Tool routes. It preserves the existing two-second cutoff and all
  unchanged quality, safety, latency, token, and authority gates.
- The separately authorized schema-v10 `SERVER_DEFAULT/gpt-5.6-sol` run
  `memory-regression-20260731t012841z-bebeac67` recalled every candidate but
  completed zero strict judge decisions. Its `146` attempted judge requests
  hit `HARD_CUTOFF`, while `49` candidate-bearing cases failed closed before
  judge egress as `RELEVANCE_ADMISSION_UNAVAILABLE`. Final Recall@5/current-
  fact accuracy was `0/0`, false injection and every authority/privacy leak
  counter were zero, and p95/p99 latency was `1856/1862 ms`. The profile failed
  unchanged gates and selected no policy.
- The independent schema-v10 `FOHWSU/deepseek-v4-flash` run
  `memory-regression-20260731t013610z-b91342e0` completed `157/195` candidate-
  bearing judge decisions, including `60` valid abstentions. Its `38` failures
  were `36` `HARD_CUTOFF` plus `2` pre-judge
  `RELEVANCE_ADMISSION_UNAVAILABLE` cases. Candidate Recall@20 remained `1.0`,
  Final Recall@5/current-fact accuracy was `0.558974/0.581818`, false injection
  and every authority/privacy leak counter were zero, and p95/p99 latency was
  `1854/1858 ms`. Quality and latency gates failed, so no policy was frozen.
- The first GPT execution initially produced no bundle because the schema-v10
  reporter inherited schema-v4/v5's rule that every candidate-bearing case
  must have `AdmissionReady=true`. The runtime had correctly failed closed
  before judge egress. Schema v10 now aggregates that bounded state only when
  rerank/judge readiness, judge token authority, Provider-sent IDs, Final,
  Injected, and prompt Memory tokens are all empty/zero. Historical schema-v4/
  v5 reporting remains strict. Both live bundles are aggregate-only, private,
  mode-`0600` evidence; transient credentials and scoped Compose state were
  destroyed. Validation and Promotion remain blocked.
- The separately authorized schema-v11 Luna Development run
  `memory-regression-20260731t034030z-07481931` completed all `300` cases and
  retained report SHA-256
  `0dfe7733005bd211664ebaa47a9a5325c0638288f90c736986756eda34a37205`.
  Candidate Recall@20 remained `1.0`, but Final Recall@5/current-fact accuracy
  fell to `0.107692/0.115152`; false injection was `1/300`, p95/p99 was
  `2853/2855 ms`, `154` cases reported `RELEVANCE_ADMISSION_UNAVAILABLE`, and
  `19` complete combination stages reported `HARD_CUTOFF`. Only `41` Luna
  requests were attempted and only `22` complete rerank-plus-judge decisions
  were obtained. The run failed unchanged gates, selected no policy, and did
  not enter Validation.
- Source tracing proved that schema v11 did not run 300 cases concurrently:
  cases were sequential. However, each candidate-bearing case ran BGE rerank
  and Luna concurrently, query embedding had a `750 ms` cutoff, the combined
  stage stopped near `2850 ms`, and cases had no cooldown. Those execution
  budgets made the failed Development result unsuitable for judging the
  underlying relevance accuracy across Provider paths with different latency.
- The owner now selects a separately versioned schema-v12 accuracy-first
  Development successor. It preserves schema-v11 evidence unchanged, removes
  application-level stage deadlines and latency pass/fail gates for this
  Development hypothesis, and executes the full path strictly serially:
  BGE query embedding -> admission -> BGE rerank -> Luna judge -> Record.
  Latency remains aggregate diagnostic evidence only. The operator may abort a
  genuinely stuck run manually; no automatic elapsed-time cutoff may convert a
  slow valid response into an accuracy failure.

## Assumptions (updated)

- The current fixed BGE-M3 embedding/rerank models remain unchanged as retrieval
  and ordering stages.
- Fixed two-stage scores, candidate margin, bilingual BGE intent, and three
  SiliconFlow candidate-aware judges are historical failed hypotheses.
- Candidate-blind `PlanTools`, first-ToolRound, and schema-v9 diagnostic routes
  are completed failed Development hypotheses. They remain default-off and are
  not the next selection authority.
- The candidate-first configured GPT and DeepSeek hypotheses were executed as
  separate Development profiles and both failed unchanged gates. Neither may
  enter Validation.
- The owner rejected the local-model successor. The next architecture
  candidate is a globally fixed cloud Memory Judge at
  `https://sub.mumubuku.top/v1`, type `openai_compatible`, model
  `gpt-5.6-luna`, regardless of the active answer model.
- The model alias does not prove the upstream parameter count or implementation.
  Evidence binds only the exact Provider identity, Base-URL hash, alias,
  adapter/prompt/decoding versions, and observed behavior.
- The successor requires new schema-v11 profile/report/criteria identities.
  It must not rewrite schema-v10 or its historical `900/1500/2000 ms` gates.
- Schema-v11 measures the complete Memory flow against p95 `<=1500 ms`, p99
  `<=2500 ms`, and a `<=3000 ms` hard cutoff. These are owner-selected product
  budgets, not an industry-standard claim.
- Every hybrid relevance policy remains default-off and non-promotional until
  it passes recall, injection, and authority gates plus a separately reviewed
  performance phase. Schema-v12 Development cannot itself authorize
  Validation or promotion because its latency evidence is diagnostic only.
- Schema v11 is now immutable failed evidence. The accuracy-first successor
  requires new schema-v12 profile/report/criteria/reader identities rather
  than retuning or overwriting schema v11.
- Schema-v12 Development prioritizes complete relevance decisions over
  latency: no application-level embedding, rerank, judge, combined-stage, or
  case deadline participates in execution or pass/fail. Provider calls and
  cases are strictly serialized; measured latency is diagnostic only until a
  later, separately reviewed performance phase.

## Open Questions

- None.

## Requirements (evolving)

### Runtime selection

- Run the fixed exact/CJK BM25/BGE vector/RRF candidate recall before relevance
  admission. Candidate recall is request-local and has no prompt, Usage, or
  reader-promotion authority.
- Reauthorize the current user, scope, revision/hash, visibility epoch,
  projection generation, validity, deletion, Sensitive policy, and source
  trust before any candidate reaches a Provider.
- Send only the deterministic secret-redacted query plus contiguous
  request-local candidate ordinals/bodies to the globally fixed
  `SERVER_DEFAULT` / `gpt-5.6-luna` Memory Judge. Never send Memory IDs,
  scores, scope, revisions, database metadata, credentials, or authority
  fields.
- Accept only the existing exact JSON schema with zero to five unique in-range
  ordinals. An empty array is `no_memory`; malformed, duplicate, out-of-range,
  late, drifted, or failed output fails closed.
- Run the globally fixed Luna judge and fixed BGE-M3 candidate rerank under the
  same bounded stage context, intersect selected ordinals with BGE order, then
  apply the existing Top-5 and 600/900-token selector.
- The answer model receives only the post-judge, post-rerank, post-
  reauthorization final set. Recalled but rejected candidates never enter its
  prompt or durable chat state.
- Preserve rerank scores only in request-local typed structures.
- Keep SQL pre-egress reauthorization and repeat current authority checks after
  every Provider boundary and before final hydration/injection.
- Return an explicit empty hybrid final set when nothing passes.
- Fail closed to no hybrid Memory on every missing/invalid/low-confidence,
  redacted, timeout, stale, or Provider-failure path; never inject unscored RRF
  or v1 fallback candidates.
- A Luna timeout, Provider error, invalid output, or protocol drift must not
  fail the normal answer. Continue chat with an explicitly empty v2 Memory set
  and never fall back to recalled, reranked, schema-v10, or other unjudged
  candidates. The only accepted degradation is loss of personalization for
  that turn.
- Never persist raw vector/rerank scores, query text, Memory content, or
  credentials in diagnostics, reports, logs, or chat metadata.
- Preserve current user/scope/revision/hash/epoch/generation reauthorization
  before and after every Provider boundary.

### Calibration and evidence

- Add a live calibration lane whose threshold grid and objective are fixed and
  configuration-hashed before Provider work.
- Choose thresholds using only the 300-case development split.
- Persist only aggregate threshold/slice counts and metrics; exclude raw
  scores, case IDs, queries, Memory content, and credentials.
- Bind the aggregate diagnostics schema/version into `configurationSha256`.
  Publish cumulative threshold counts, fixed-grid failure counts, and
  safety-first/recall-first attempts even when no scalar pair is feasible.
- Bind the exact intent anchor version/SHA-256, intent selection objective,
  `[-1.00,1.00]` step-`0.01` grid, and conservative cost authority into
  `configurationSha256` before Provider construction.
- The intent Provider call may contain only the secret-redacted query and two
  fixed non-user anchors. Missing, invalid, drifted, late, or low-margin intent
  evidence sends zero Memory documents and returns `no_memory`.
- Add a schema-separated cloud-judge Development lane with a fixed Provider,
  model, prompt/schema SHA-256, owner-authorization policy ID, deterministic
  decoding rules, and cost authority hashed before Provider construction.
- Retain every cloud-judge report as immutable historical failed evidence, but
  do not use that lane as the next selection authority.
- Add a schema-separated configured-main-model candidate-judge Development
  lane. Hash the exact Provider type/identity/base-URL hash/model, strict judge
  prompt/schema/decoding and adapter versions, cost authority, BGE tuple, and
  unchanged evaluation criteria before Provider construction.
- Preserve schema-v7 and both empty schema-v8 attempts as immutable historical
  evidence. A new failure-subtype run uses profile/report schema v9, reader v7,
  `route_complete_retrieval_fail_closed_v1`, and the hash-bound
  `memory-tool-route-failure-taxonomy-v1`; bounded route subtype counts must
  equal route failures, retrieval-incomplete aggregate counts must reconcile,
  and no raw Provider error/body may be retained.
- Preserve schema-v9 as immutable historical evidence and perform no further
  Tool-route paid diagnostics. It cannot select or influence the new policy.
- Add a schema-v11 Development lane rather than mutating schema v10. Bind the
  exact `SERVER_DEFAULT`, `openai_compatible`, Base-URL hash,
  `gpt-5.6-luna`, strict judge adapter/prompt/decoding identities, BGE tuple,
  owner-egress policy, criteria version, and cost authority before Provider
  construction.
- Measure p95/p99 and hard cutoff over the complete Memory flow for schema v11,
  using exact budgets `1500/2500/3000 ms`. A single protocol smoke is not
  percentile evidence and cannot unlock Validation.
- Add schema v12 without changing schema v11. For schema-v12 Development,
  execute BGE query embedding, local admission, BGE rerank, Luna judge, and
  Record in strict sequence with at most one Provider request in flight. Do
  not apply application-level stage or case deadlines; retain aggregate
  complete-flow and per-stage latency only as diagnostics, never as an
  accuracy-first pass/fail criterion. After each complete case, wait a fixed
  `1s` cooldown before starting the next case; include that policy in the
  configuration identity, but exclude cooldown from model-flow latency.
- For an explicit `429`, `408`, `5xx`, or retryable transport interruption,
  retry the failed Provider request once and strictly serially. Respect a valid
  `Retry-After`; when it is absent, wait `5s`. Do not retry invalid judge JSON,
  schema/protocol drift, deterministic `4xx`, or any other error. Bind the
  retry policy/version into the schema-v12 configuration identity.
- Score a relevant case as final only when the strict candidate judge selects
  an ordinal that survives unchanged BGE ordering and final authority. Score a
  non-empty final set for an unrelated-negative continuation as false
  injection.
- The schema-v10 GPT and DeepSeek runs remain separate historical Development
  hypotheses; neither result authorizes the schema-v11 Luna profile.
- Keep `unauthorizedProviderEgressCount=0`: under the explicit owner policy,
  ordinary irrelevant candidates are authorized processing, while every
  forbidden exclusion reason still counts as unauthorized. False injection
  remains unchanged and must pass independently.
- Freeze the selected policy/version in code before validation.
- Evaluate the frozen policy on the 100-case validation split without retuning.
- Keep the visible machine `holdout` explicitly non-formal and
  `promotionEligible=false`.
- Do not weaken Candidate Recall@20, Final Recall@5, current-fact, false
  injection, Provider-egress, latency, token, or authority gates. Historical
  relative-cost evidence remains immutable; the explicit owner-budget profile
  replaces that profile's relative ratio selection gate with exact absolute
  preauthorization rather than fabricating a favorable denominator.

### Compatibility and rollout

- Keep v1 as the only prompt/Usage authority and all hybrid flags default-off.
- Version the selector/calibration policy and bind it into
  `configurationSha256`; do not silently change behavior under an indistinct
  reader configuration.
- Preserve deterministic fake-provider, PostgreSQL 17, publication, secret
  scan, interruption, and total teardown tests.
- Update the stale live-comparison tracking status with the completed run and
  its failed candidate outcome, without committing private evidence.
- Development may reuse transient mode-`0600` decrypted copies of the existing
  Server Vault credentials only under the owner's explicit authorization. The
  copies must be overwritten and removed after each run, and the runner must
  never inspect/decrypt the Vault. Validation still requires fresh,
  independently authorized credentials for BGE and the named judge Provider.
- The owner explicitly authorized this transient Server Vault export for the
  schema-v11 Development run after offline gates pass. This grants no Vault
  access to the runner and no credential or execution authority to Validation
  or production composition.
- The owner separately authorized the same operator-only transient export for
  the existing `RAG:SILICONFLOW` credential needed by the fixed
  `Pro/BAAI/bge-m3` and `Pro/BAAI/bge-reranker-v2-m3` stages. The BGE and Luna
  input files must remain independent, mode `0600`, read-only in the runner,
  rejected when linked or byte-equal, and overwritten/removed on every exit.
- The owner conditionally authorized one real 300-case schema-v11 Development
  run using Luna and SiliconFlow quota, but only after schema-v11 offline,
  race, PostgreSQL, Compose, and secret-scan gates all pass. The run must stop
  on failed metrics without changing criteria and grants no Validation,
  production-composition, or promotion authority.
- Even if every Development gate passes, execution must stop and present the
  aggregate report for owner review. The 100-case Validation run requires a
  later explicit authorization and can never start automatically from the
  Development result.
- Even a later passing Validation result grants no production authority. The
  fixed Luna reader remains default-off until a separate deployment review
  verifies the exact credential binding, default flags, rollback path,
  monitoring, and operational readiness, followed by explicit owner promotion
  authorization.

## Acceptance Criteria (evolving)

- [ ] An unrelated query may produce private candidates, but the strict judge
  returns an empty ordinal set and zero hybrid final/injected Memory; no
  rejected candidate body reaches the answer model.
- [ ] Retrieval failure, empty final selection, stale final hydration, or full
  post-hydration redaction produces no Memory body and normal chat continues.
- [ ] Relevant multilingual/paraphrase cases retain Candidate Recall@20
  `>=0.95`, Final Recall@5 `>=0.90`, and current-fact accuracy `>=0.95` on the
  frozen validation split.
- [ ] False injection is `<=0.02`, with zero cross-user, deleted, secret,
  untrusted-source, and policy-unauthorized Provider-egress events.
- [ ] Missing, duplicate, NaN/Inf/out-of-range, stale, timeout, redacted, and
  Provider-failure signals fail closed without changing normal chat success.
- [ ] Schema-v11 complete-flow latency is p95 `<=1500 ms`, p99 `<=2500 ms`,
  and every case is `<=3000 ms`; judge-only timing is diagnostic, not the
  acceptance metric.
- [ ] Schema-v12 accuracy-first Development has at most one Provider request
  in flight, no application-level stage/case cutoff, and no latency pass/fail
  gate; every non-empty-candidate case reaches a complete rerank-plus-judge
  decision unless the Provider returns an explicit error or the operator
  manually interrupts the run.
- [ ] Schema-v12 waits `1s` between cases, starts no next Provider request
  during that cooldown, and reports cooldown separately from measured
  complete-flow latency.
- [ ] Schema-v12 retries only `429`, `408`, `5xx`, and retryable transport
  interruptions, at most once, respecting `Retry-After` or otherwise waiting
  `5s`; the retry remains serialized and all other errors fail closed without
  retry.
- [ ] Raw scores and plaintext are absent from PostgreSQL observations,
  retained artifacts, logs, Docker metadata, and Git.
- [ ] Calibration rejects validation/holdout tuning and publishes only
  aggregate mode-`0600` evidence under exclusive paths.
- [ ] Existing v1 prompt/Usage behavior is byte-compatible and hybrid remains
  non-promotional/default-off.
- [ ] Focused race tests, PostgreSQL 17 replay, all backend tests, `go vet`,
  Compose/preflight, image build, and standalone full verification pass.

## Definition of Done

- The exact candidate-judge prompt/schema/decoder are shared by capture and
  runtime adapters rather than copied into the evaluator.
- Policy selection is reproducible, versioned, split-safe, and contains no raw
  score/plaintext artifact.
- One independently authorized validation run records truthful results; a
  failed gate remains a valid failure and never triggers promotion.
- Contracts, operator docs, tracking state, tests, rollback, and code agree.
- Changes are committed in focused batches and the task is archived only after
  final verification.

## Technical Approach

1. Keep candidate retrieval, judge/BGE intersection, final selection, and
   current-authority recording in `usermemory`. Retain
   `HybridMemoryToolRouter` only to replay historical failed route evidence.
2. Reuse `BuildHybridCandidateJudgePrompt`, the exact JSON ordinal decoder,
   and `memoryjudge.ChatAdapter`; do not create a second prompt or Provider
   transport.
3. Add a schema-separated configured-main-model candidate-judge Development
   profile. Bind exact Provider ID/type/base-URL hash/model and the shared
   judge/BGE/cost/egress contracts before constructing either Provider.
4. Execute candidate recall before admission. Run the strict configured-model
   judge and fixed BGE reranker concurrently only after current-authority
   checks and redaction, intersect their results, and record no final set on
   any incomplete path.
5. Record first, then hydrate only the exact final lane through migration
   `065`; repeat current authority and redaction before any future product
   injection. The Development lane itself remains counterfactual and cannot
   mutate prompt/Usage authority.
6. Preserve schema-v4/v5 candidate-judge and schema-v6/v7/v8/v9 Tool-route
   evidence byte-for-byte. The new profile receives new schema/reader/adapter
   identities and cannot inherit their results.
7. Require independent mode-`0600` BGE and configured-judge credentials for a
   live run, scan both secrets from retained surfaces, and destroy all scoped
   Docker/runtime state on every exit path. The authorized GPT and DeepSeek
   executions followed this boundary and retained only private aggregate
   evidence.
8. Use `fake_protocol` only for deterministic protocol/lifecycle proof. Treat
   GPT and DeepSeek as separate live Development hypotheses; both failed, so
   freeze and Validation remain unavailable.

Current checkpoint: product first-round Tool Loop, migration-065 final
hydration, schema-v7 Development adapter/profile/report, focused tests,
regression lifecycle tests, and PostgreSQL 17 final-hydration replay exist.
Historical schema-v6 GPT failed; the first DeepSeek run is protocol-invalid;
corrected DeepSeek Flash failed. The first schema-v7 GPT Development profile
and the independent DeepSeek Flash profile also failed. No policy is frozen,
Validation is blocked, and the runtime flag remains default-off.

The schema-v9 route-only diagnostic successor is implemented, fake-protocol
verified, and live-executed once with separate owner authorization. It does not
reinterpret either failed v7 run or either empty v8 attempt. The v9 artifact
retained `31` context-deadline, `83` invalid-Tool-Call, and `174` unclassified
router failure categories plus `174` independent retrieval-incomplete counts.
It failed unchanged quality/latency/cutoff gates with zero authority/privacy
leaks and selected no policy. Credentials and isolated runtimes were fully
destroyed. The production 750 ms embedding cutoff, two-second hard cutoff, and
no-retry behavior remain unchanged. No further paid run is authorized.

The offline lifecycle follow-up now closes every started route before the
capture case finishes and rejects previous-generation Recorder writes. Focused
route/admission/cancellation tests and the regression topology gate pass
without Provider traffic. The retained v9 evidence and failed selection state
remain unchanged.

The schema-v10 configured candidate-judge Development lane is implemented and
has now been executed against both authorized configured Provider profiles.
Profile config v10, reader v8, report v10,
cost-basis v6, exact Provider/adapter authorization, independent credential
handling, fake CLI/Compose topology, aggregate publication, and teardown are
covered. A PostgreSQL 17 `fake_protocol` replay executed all 300
Development cases, retained only the expected private two-file failed-metric
bundle, and destroyed every scoped runtime object. Focused race, all backend,
`go vet`, and standalone full gates passed before live execution. Historical
profile/cost JSON bytes for v4/v5/v6/v7/v9 match `HEAD`. The real GPT and
DeepSeek Development profiles both retained valid failed-gate evidence and
selected no policy. Validation, production composition, and promotion remain
unrun and blocked.

The evaluated schema-v10 policy is the candidate-first contract in
[`research/candidate-first-admission-reset.md`](research/candidate-first-admission-reset.md):

1. Recall current-authorized candidates before admission.
2. Give the exact configured GPT or DeepSeek judge only redacted query and
   ordinal candidate bodies; accept an empty or strict bounded ordinal set.
3. Intersect selected ordinals with the unchanged BGE-M3 rerank/Top-5/token
   pipeline and reauthorize before final hydration.
4. Evaluate each exact Provider/model on Development with aggregate-only,
   split-safe evidence and unchanged gates.
5. Keep the deployed default v1 prompt/Usage path byte-authoritative and fail
   closed to normal chat without v2 Memory on every uncertain path.

Both exact configured-model profiles failed. The owner rejected the earlier
Option B local-model recommendation. The fixed Luna schema-v11 profile is the
next design candidate and requires a separate implementation/evidence
contract.

## Implementation Plan

1. Add transient pre-rerank and post-rerank score carriers, fail-closed
   selection, policy versioning, and focused Go tests.
2. Extend the isolated native runner with aggregate-only development
   calibration, strict split admission, private publication, and teardown
   tests.
3. Record the completed fixed-grid Development failure, add schema-v2
   aggregate diagnostics, then rerun Development with a fresh separately
   authorized Key.
4. Implement the query-only bilingual intent margin gate and evaluate its
   fixed 201-threshold grid on Development only; freeze it in code only when
   all unchanged gates pass.
5. Record the no-feasible schema-v3 result and keep Validation denied.
6. Implement the owner-authorized cloud candidate judge, policy-aware egress
   scoring, strict ordinal output, concurrent BGE/judge execution, and a new
   Development-only capture profile.
7. Record all three failed judge runs, cancel the unrun Qwen3.5 hypothesis, and
   implement the exact main-model `search_memory` Tool-route Development lane
   for separately configured GPT and DeepSeek Providers.
8. Record the failed GPT run, protocol-invalid DeepSeek Pro run, and corrected
   failed DeepSeek Flash run; keep Validation and Promotion blocked.
9. Replace the separate `PlanTools` preflight with the first-round chat Tool
   Loop, add post-Record final hydration, and version the Development lane as
   schema-v7 without rewriting schema-v6 evidence.
10. Complete offline/PostgreSQL/Compose/full gates. Only after an exact live
    schema-v7 Development profile passes may one fresh independently authorized
    Validation capture run proceed without retuning.
11. Update benchmark/hybrid/Tool Loop contracts, operator workflow, and live
   tracking; retain only sanitized mode-`0600` evidence and leave promotion
   disabled.
12. Classify the collapsed first-round failures through a schema-v8,
    aggregate-only diagnostic lane. Do not infer historical v7 subtypes and do
    not run the paid lane without fresh explicit authorization.
13. Preserve both zero-artifact schema-v8 attempts, then version a schema-v9
    route-only diagnostic that retains bounded route categories while counting
    fail-closed retrieval incompleteness separately. Do not change cutoffs,
    retry, or promotion authority, and require fresh authorization before any
    third paid run.
14. Stop the candidate-blind route line. Add a new configured-main-model
    candidate-judge Development profile that reuses the existing strict judge
    and hybrid execution, proves fake/offline/PostgreSQL behavior, and performs
    no live Provider call without fresh authorization.
15. Execute the separately authorized GPT and DeepSeek schema-v10 Development
    profiles, retain both failed-gate bundles, aggregate strictly empty pre-
    judge retrieval failures without weakening historical schemas, and keep
    Validation/Promotion blocked.
16. Replace the rejected local-model successor with a separately versioned
    schema-v11 fixed cloud Memory Judge profile for
    `SERVER_DEFAULT/openai_compatible/sub.mumubuku.top/gpt-5.6-luna`. Keep the
    strict prompt/decoder and BGE tuple, adopt the owner-selected complete-flow
    `1500/2500/3000 ms` latency criteria without rewriting schema v10, pass a
    one-request protocol smoke, then require separate authorization before the
    300-case Development run.
17. Preserve the failed schema-v11 bundle and implement schema-v12 as a new
    accuracy-first Development profile: remove application stage/case
    deadlines and latency gating, serialize BGE embedding, admission, BGE
    rerank, Luna judge, and Record, add a fixed `1s` inter-case cooldown plus
    one bounded transient retry (`Retry-After`, otherwise `5s`), and prohibit
    any live paid execution until the revised offline gates pass and the owner
    separately authorizes it.

## Decision (ADR-lite)

**Context:** Ungated BGE already has perfect Development final recall/current-
fact accuracy, while every false injection is an unrelated-negative turn.
Scalar/BGE intent gates and three independent candidate judges failed. Passing
all candidates to the answer model before selection would redefine injection.

**Historical decision:** Use the current selected GPT or DeepSeek model's exact
`search_memory` Tool decision as the next Development abstention policy. That
hypothesis was implemented and failed; it is retained only as history.

**Consequences:** Memory-relevant turns may require a normal same-model Tool
continuation round, but no hidden chat model or judge Provider is added. The
policy becomes Provider/model-specific and must be revalidated on model or
Tool-contract drift. v1 remains production authority until every gate passes.

**2026-07-30 amendment:** The first implementation used `PlanTools` as a
separate Provider preflight and failed live Development. Preserve those results
as failed evidence, but do not promote that request shape. The intended
architecture is one first chat Tool round containing Memory beside the other
read-only tools, followed by same-model continuation only when called.

**2026-07-30 implementation amendment:** That first-round architecture is now
implemented behind a default-off flag with migration-065 final hydration.
Schema-v7 measures the new request shape separately. Its offline evidence
passes and its first GPT live Development profile failed unchanged gates; it
cannot inherit any schema-v6 result.

**2026-07-30 diagnostic amendment:** Both schema-v7 profiles failed and the
DeepSeek report collapsed `263` route failures into one code. Keep those files
immutable. A new v8 diagnostic taxonomy may measure only bounded aggregate
failure categories and can never select a policy or unlock Validation.

**2026-07-30 completeness amendment:** Two schema-v8 runs produced no artifact;
the second bounded failure was `admission_state`. Preserve both attempts.
Schema v9 measures route completeness independently from fail-closed retrieval
completeness, retains no case identity/plaintext/raw error, and still cannot
select a policy or unlock Validation/Promotion.

**2026-07-31 architecture reset:** Candidate-blind routing cannot discover
implicit personalization and its live Tool/SSE decision path failed unchanged
quality/reliability gates. Separate candidate recall from prompt injection.
Recall current-authorized candidates first, then use the exact configured GPT
or DeepSeek model through the existing strict candidate-aware ordinal judge.
Only ordinals that also survive BGE ordering and post-Provider authority may
enter a future answer prompt. Preserve every Tool-route artifact, keep all
runtime flags default-off, and implement only Development/fake/offline support
until a fresh live run is explicitly authorized.

**2026-07-31 live outcome:** The owner separately authorized exact schema-v10
GPT and DeepSeek Development runs. GPT completed no strict judge decision;
DeepSeek completed most decisions but reached only `0.558974` Final Recall@5
and `0.581818` current-fact accuracy. Both exceeded the latency criterion while
retaining zero false injection and zero authority/privacy leaks. Neither
profile may enter Validation.

**2026-07-31 successor amendment:** The owner rejected local inference and
selected one globally fixed cloud Memory Judge independent of the answer
model: `SERVER_DEFAULT`, `openai_compatible`,
`https://sub.mumubuku.top/v1`, `gpt-5.6-luna`. This is schema v11, not a
schema-v10 retune. The owner selected complete-flow latency budgets p95
`1500 ms`, p99 `2500 ms`, and hard cutoff `3000 ms`. A one-request live smoke
accepted the exact `temperature=0`, `enable_thinking=false`, max-output-128
OpenAI-compatible request, returned strict schema-v1 ordinal JSON, selected
only the directly useful candidate despite an injected candidate body, and
completed in `2354 ms`. That proves protocol compatibility and the hard-cutoff
path only; it is not percentile or Development quality evidence.

**2026-07-31 accuracy-first amendment:** The authorized schema-v11 Development
run completed but failed because only `22/195` candidate-bearing cases obtained
a complete rerank-plus-judge decision under the short execution budgets. The
owner rejects using cross-Provider latency as an accuracy verdict and rejects
intra-case Provider concurrency for the successor. Schema v11 remains immutable
failed evidence. Schema v12 will run the full Provider path strictly serially,
without application-level elapsed-time cutoffs or a latency acceptance gate;
latency is diagnostic only and Validation remains blocked.

**2026-07-31 offline verification:** Schema v12 now passes focused race, all
backend, `go vet`, regression-topology, PostgreSQL 17 `fake_protocol`, frontend,
RAG, and changed-surface security checks. The fake run produced the expected
private two-file failed-metric bundle with 300 query attempts, 195 serial
rerank-plus-judge decisions, 299 virtual cooldowns, and complete teardown. Two
monolithic full-standalone attempts passed their structure/topology stages but
were interrupted by a Docker Desktop WSL integration proxy crash; no full-pass
claim is made. At this offline checkpoint no schema-v12 live Provider call had
been authorized or executed.

**2026-07-31 schema-v12 live outcome:** The owner separately authorized and
the runner completed exactly one 300-case accuracy-first Development run.
It completed all `195` candidate-bearing rerank-plus-judge decisions with zero
failed cases, `203` Luna attempts including `8` bounded retries, and all `299`
real cooldowns. Candidate Recall@20 was `1.0`, Final Recall@5 was `0.974359`,
and current-fact accuracy was `0.969697`, but `29` false-injection cases raised
the rate to `0.096667` against the unchanged `0.02` maximum; the `stable_fact`
current-fact slice also failed. Safety/authority leaks stayed zero and prompt
budgets passed. Report SHA-256 is
`126536772d71a5815f1cb6029deb568d0655c8780924ac0428951807975c8011`.
The report selected no policy, all transient credentials/runtime objects were
destroyed, and Validation/production/promotion remain blocked.

**2026-07-31 regression-contract repair decision:** Offline review proved that
the v2-regression `unrelated_negative` query asks whether an unrelated record
should influence the answer while the only current-authorized candidate says
that it has no bearing. That candidate is literally useful under prompt v1,
although the evaluator requires no Memory. The owner approved repairing the
machine regression corpus before changing the model or prompt. Preserve the
existing v2 generator, protected bytes, hashes, and every report unchanged.
Add a separately named v3 regression generator/root whose unrelated-negative
query is a genuine user task and whose same-entity/same-scope hard-negative
Memory is topically similar but cannot answer it. Keep the 500-case
distribution, draft/non-promotional status, criteria, strict audit, and
provider-free generation/verification boundaries unchanged. This decision
authorizes only offline code/tests and a new disposable generated bundle; it
does not authorize another live capture, Validation, or promotion.

**2026-07-31 regression-contract repair implementation:** The separately
seeded `memory-regression-zh-mixed-v3` generator now produces a normal
agenda-heading query plus a same-entity/same-scope weather-board hard negative,
while the historical v2 generator remains byte-identical under pinned raw and
content hashes. Exact generator/auditor/ID dispatch rejects unknown or mixed
v2/v3 artifacts, and the v3 semantic audit rejects a return to self-referential
negative wording. The private disposable four-file bundle replayed exactly at
`0700/0600`; focused race tests, all backend Go tests, and `go vet ./...`
passed. The committed v3 status is aggregate/hash-only. No Docker, Provider,
Validation, production, or promotion action was executed.

**2026-07-31 v3 capture integration preflight:** Source tracing confirmed that
the existing wrapper already accepts an explicit
`--regression-root <protected-root>` while retaining v2 as its compatibility
default. The protected loader now has direct v2/v3 replay coverage, both pools
have Development/Validation split-selection coverage with machine Holdout
rejection, and schema-v12 profile construction proves distinct v2/v3
configuration SHA-256 values. Focused race tests, all backend Go tests,
`go vet ./...`, and wrapper shell syntax passed. This is offline preflight only;
no v3 model request or quality result exists yet. The retained exact v8 cost
basis replayed with raw SHA-256
`5d5c33e807185170fa52080349c8875f28c1313be2d64344f8dc3c31ec99e6c8`
and canonical SHA-256
`d75a6edf7fd5f050c3e30c4cae5960972a8e6065676f477321a5510ad7e5dd47`.
Under that same live Provider/cost authority, historical v2 configuration
`bd0fa42e0b612da39d974a06027945e831cfce48cabd9226a1bc06b76aad2b16`
separates from the proposed v3 configuration
`72940f138ba53dda01e5eddad5e82bf05e2740fd671549e2310adea61a1bf49f`.

**2026-07-31 v3 live Development outcome:** The owner separately authorized
exactly one v3 300-case accuracy-first Development run. Run
`memory-regression-20260731t093606z-89719a18` used configuration SHA-256
`72940f138ba53dda01e5eddad5e82bf05e2740fd671549e2310adea61a1bf49f`
and completed with zero failed cases, all `195` candidate-bearing reranks,
`202` Luna attempts including `7` bounded retries, and all `299` real
cooldowns. Candidate Recall@20/Final Recall@5/current-fact accuracy was
`1.0/0.984615/0.981818`. The repaired corpus reduced false injection from the
historical v2 result's `29/300` to `10/300`, but `0.033333` still exceeded the
unchanged `0.02` maximum; the `stable_fact` current-fact slice also failed at
`0.933333`. Every safety/authority counter remained zero and prompt budgets
passed. Report SHA-256 is
`f35cfea03c98de4ecfff8ea9c774fbcef706f895da9db3a72d606e99efee2eb7`;
manifest SHA-256 is
`5be7db8903c5e26cd2dcadae12cde1a3c52f3421bb46862db481e8105e955176`,
binding canonical cost SHA-256
`d75a6edf7fd5f050c3e30c4cae5960972a8e6065676f477321a5510ad7e5dd47`.
No policy was selected. Temporary credentials, the supervisor, and every
isolated Compose object were destroyed; the base PostgreSQL container remained
stopped. Validation, production, promotion, and an automatic rerun remain
blocked.

## Expansion Sweep

- Future evolution: evaluate the fixed Luna candidate-aware profile under the
  new schema-v12 accuracy-first identity. Do not add benchmark-tuned query
  rules, local-model fallback, or answer-model-specific routing.
- Related scenarios: keep L2 Scene/L3 Persona and active-reader promotion out
  of this change; they may consume a passing L1 policy later.
- Failure/edge cases: fail closed on score/model/policy drift, redaction,
  timeout, Provider failure, stale authority, and interrupted calibration.

## Out of Scope

- Promoting v2 or changing the active reader/prompt/Usage pointer.
- Enabling `MEMORY_TOOL_LOOP_ENABLED` or any configured-model candidate judge
  in a deployed environment from this Development-only work.
- Treating forbidden exclusion reasons as cloud-authorized, or silently
  enabling cloud processing without the exact owner-policy profile.
- Tuning on the machine-visible holdout or claiming formal human-reviewed
  Holdout evidence.
- Persisting raw scores for convenience.
- Replacing BGE-M3, adopting Hindsight/Mem0/Graphiti, or changing extraction.
- L2 Scene/L3 Persona threshold tuning.

## Research References

- [`research/relevance-abstention-design.md`](research/relevance-abstention-design.md)
- [`research/cloud-judge-model-followup.md`](research/cloud-judge-model-followup.md)
- [`research/main-model-memory-tool-routing.md`](research/main-model-memory-tool-routing.md)
- [`research/memory-tool-route-failure-diagnostics.md`](research/memory-tool-route-failure-diagnostics.md)
- [`research/candidate-first-admission-reset.md`](research/candidate-first-admission-reset.md)
- [`research/fixed-luna-candidate-judge.md`](research/fixed-luna-candidate-judge.md)
- [`research/pre-judge-report-aggregation-retrospective.md`](research/pre-judge-report-aggregation-retrospective.md)
- `.trellis/spec/backend/memory-v2-benchmark.md`
- `.trellis/spec/backend/memory-v2-hybrid-shadow.md`
- `mm-chat/docs/contracts/memory-benchmark-workflow.md`

## Technical Notes

- Primary code seams: `backend/internal/usermemory/hybrid_shadow.go`,
  `backend/internal/usermemory/types.go`, the hybrid PostgreSQL repository,
  `backend/internal/memorycapture`, `cmd/memory-regression-capture`, and
  `scripts/run-memory-regression.sh`.
- The schema-v3 temporary Key was destroyed after the completed failed-gate
  run; retained evidence is aggregate-only and mode-`0600` outside Git.
- The owner authorized Server Vault credential reuse for schema-v6 Development.
  All transient decrypted mode-`0600` input files were overwritten and removed;
  retained evidence is aggregate-only and outside Git.
- The three schema-v6 run directories are mode `0700`, their two retained files
  are mode `0600`, and cleanup left zero temporary regression containers,
  networks, volumes, or decrypted credential files.
- The schema-v7 GPT run retained only its two aggregate mode-`0600` artifacts.
  Both transient Server Vault copies were overwritten and removed, and cleanup
  left zero scoped containers, networks, volumes, or operator export files.
- The schema-v7 DeepSeek Flash run retained the same two-file aggregate-only
  shape at mode `0600`. Its independent transient Vault copies were overwritten
  and removed, and cleanup again left zero scoped containers, networks,
  volumes, temporary regression directories, or operator export files.
- The schema-v9 DeepSeek Flash diagnostic retained its report and manifest as
  mode-`0600` aggregate-only evidence in a mode-`0700` external directory. The
  manifest binds configuration
  `13cc65b47ff7c358935ebd3bb1080412784e353ebc72503963b2822d9990c14f`
  and canonical cost content
  `b54b6fcfb62a33b31ef17cfd9876d392a20ef21bd25d19f67902350f194b1742`;
  the private source cost file remains separately bound by raw-file SHA-256
  `4d3fe6b0dbbc1ed80f717ae2488ce8d2a141db24dc1192a5f260f57410c3531b`.
  All transient credentials/helpers and scoped Compose objects were destroyed.
- The schema-v10 GPT report SHA-256 is
  `931228006b5f48b500cfdb56ac4a72ef8e8fa08f25d9d2c6c841ace8e34e2c7f`;
  the DeepSeek report SHA-256 is
  `c72874e9d0e11c34a88aa9a22b3c02924b8ec9fde9c0bcb0461d5c53fdc9d95a`.
  Each private run directory is mode `0700` with exactly two mode-`0600`
  aggregate artifacts. Separate transient Vault copies were overwritten and
  removed after each run, and no scoped container, network, volume, helper,
  export, or decrypted credential file remained.
- The Luna protocol smoke used the actively configured Server Vault record,
  never placed plaintext credentials in argv/environment/output, retained no
  raw Provider response, and shredded its transient encrypted-envelope copy.
  No helper, credential, or temporary smoke file remained after execution.

# Native v2 relevance abstention

## Goal

Add a versioned, split-calibrated abstention policy to the native v2 hybrid
Memory reader so the user-selected GPT or DeepSeek chat model decides whether
to call a read-only `search_memory` Tool before seeing Memory bodies. Preserve
BGE-M3 retrieval/rerank and the current v1 prompt authority until independent
evidence passes.

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

## Assumptions (updated)

- The current fixed BGE-M3 embedding/rerank models remain unchanged as retrieval
  and ordering stages.
- Fixed two-stage scores, candidate margin, bilingual BGE intent, and three
  SiliconFlow candidate-aware judges are historical failed hypotheses.
- The query-only `PlanTools` preflight is a completed failed Development
  hypothesis. The next hypothesis is an exact `search_memory` decision inside
  the existing first `ToolRoundProvider` round, evaluated separately for each
  Provider/model.
- Every hybrid relevance policy remains default-off and non-promotional until
  it passes unchanged recall, injection, latency, and authority gates.

## Open Questions

- None.

## Requirements (evolving)

### Runtime selection

- Add a versioned main-model Memory Tool-routing policy to the existing first
  `ToolRoundProvider` request. Before a valid Tool Call, the Provider sees the
  normal conversation context and exact read-only `search_memory` definition,
  never Memory candidates or bodies. Do not prepend an independent
  `PlanTools` request.
- Accept either no Tool Call (`no_memory`) or one exact, bounded
  `search_memory` call. Unknown tools, duplicates, malformed arguments, model
  or contract drift, late output, and Provider failure fail closed.
- After a valid Tool Call, run the fixed BGE-M3 embedding/RRF/rerank path and
  existing Top-5 plus 600/900-token selector. The bounded result is eligible
  only for same-Provider/same-model continuation.
- Permit speculative overlap of routing and BGE work only when candidates
  remain request-local until the Tool Call succeeds and measured latency
  includes the real decision boundary.
- Preserve rerank scores only in request-local typed structures.
- Keep SQL pre-egress reauthorization and repeat current authority checks after
  every Provider boundary and before building the Tool result.
- Return an explicit empty hybrid final set when nothing passes.
- Fail closed to no hybrid Memory on every missing/invalid/low-confidence,
  redacted, timeout, stale, or Provider-failure path; never inject unscored RRF
  or v1 fallback candidates.
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
- Add a schema-separated main-model Tool-route Development lane. Hash the exact
  Tool definition/version, adapter behavior, Provider type and identity,
  model, cost authority, BGE tuple, and unchanged evaluation criteria before
  Provider construction.
- Preserve schema-v7 and both empty schema-v8 attempts as immutable historical
  evidence. A new failure-subtype run uses profile/report schema v9, reader v7,
  `route_complete_retrieval_fail_closed_v1`, and the hash-bound
  `memory-tool-route-failure-taxonomy-v1`; bounded route subtype counts must
  equal route failures, retrieval-incomplete aggregate counts must reconcile,
  and no raw Provider error/body may be retained.
- Score a relevant case as final only when a valid Tool Call is followed by the
  unchanged BGE final set. Score a Tool Call whose non-empty BGE final set would
  enter an unrelated-negative continuation as false injection.
- GPT and DeepSeek are separate named Development hypotheses; one model's
  result cannot authorize the other.
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
  independently authorized credentials for BGE and the named route Provider.

## Acceptance Criteria (evolving)

- [ ] An unrelated query produces no `search_memory` Tool Call and zero hybrid
  final/injected Memory; no candidate body reaches the answer model.
- [ ] Retrieval failure, empty final selection, stale final hydration, or full
  post-hydration redaction produces no Memory body and normal chat continues.
- [ ] Relevant multilingual/paraphrase cases retain Candidate Recall@20
  `>=0.95`, Final Recall@5 `>=0.90`, and current-fact accuracy `>=0.95` on the
  frozen validation split.
- [ ] False injection is `<=0.02`, with zero cross-user, deleted, secret,
  untrusted-source, and policy-unauthorized Provider-egress events.
- [ ] Missing, duplicate, NaN/Inf/out-of-range, stale, timeout, redacted, and
  Provider-failure signals fail closed without changing normal chat success.
- [ ] Raw scores and plaintext are absent from PostgreSQL observations,
  retained artifacts, logs, Docker metadata, and Git.
- [ ] Calibration rejects validation/holdout tuning and publishes only
  aggregate mode-`0600` evidence under exclusive paths.
- [ ] Existing v1 prompt/Usage behavior is byte-compatible and hybrid remains
  non-promotional/default-off.
- [ ] Focused race tests, PostgreSQL 17 replay, all backend tests, `go vet`,
  Compose/preflight, image build, and standalone full verification pass.

## Definition of Done

- The exact Tool definition and route decision contract are shared with the
  product Tool Loop rather than copied into the evaluator.
- Policy selection is reproducible, versioned, split-safe, and contains no raw
  score/plaintext artifact.
- One independently authorized validation run records truthful results; a
  failed gate remains a valid failure and never triggers promotion.
- Contracts, operator docs, tracking state, tests, rollback, and code agree.
- Changes are committed in focused batches and the task is archived only after
  final verification.

## Technical Approach

1. Keep hybrid retrieval and final authority in `usermemory`; keep the typed
   `HybridMemoryToolRouter` only as a Development compatibility seam.
2. Own the exact no-argument `search_memory` definition, JSON hash, and call
   validation in `internal/chat`. `internal/memoryroute` delegates to that
   authority instead of copying it.
3. Add `search_memory` to the existing first `ToolRoundProvider` request behind
   `MEMORY_TOOL_LOOP_ENABLED=false`. Buffer first-round text/reasoning, accept
   only an exact first-round call, remove Memory from later rounds, and continue
   on the same Provider/model.
4. After a valid product call, execute the fixed BGE embedding/RRF/admission/
   rerank/Top-5/token path without v1 or `MarkUsed`. Record first, then hydrate
   the exact final lane through migration `065`, repeat current authority, and
   redact bodies again before the Tool Result.
5. On retrieval/continuation failure, preserve ordinary chat: empty/failure
   results continue without Memory, and a pre-content continuation failure
   recovers from the original request without any Memory body.
6. Preserve schema-v6/profile-v6/cost-basis-v4 as immutable failed preflight
   evidence. Add schema-v7/profile-v7/cost-basis-v5 with reader v5, adapter
   `chat-first-tool-round-memory-decision-v1`, and artifact
   `memory-first-tool-round-development.json`.
7. Require two independent mode-`0600` credential inputs for live capture,
   scan both secrets from retained surfaces, and destroy all project-scoped
   Docker/runtime state on every exit path. Development may use explicitly
   authorized transient Server Vault copies; Validation may not.
8. Use `fake_protocol` only for deterministic protocol/lifecycle proof. Run GPT
   and DeepSeek as separate live Development hypotheses; freeze and add a
   matching Validation lane only after one exact profile passes all unchanged
   gates.

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

Use the selected main-model Tool route from
[`research/main-model-memory-tool-routing.md`](research/main-model-memory-tool-routing.md):

1. Expose an exact read-only `search_memory` Tool to the user-selected GPT or
   DeepSeek model without sending Memory candidates.
2. Treat no call as `no_memory`; validate one exact call before releasing any
   bounded BGE result to same-model continuation.
3. Reuse the unchanged BGE-M3 embedding/RRF/rerank/Top-5/token pipeline after
   current-authority checks.
4. Evaluate the exact route decision on Development with aggregate-only,
   split-safe evidence and unchanged gates.
5. Keep the deployed default v1 prompt/Usage path byte-authoritative and fail
   closed to normal chat without v2 Memory on every uncertain path.

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

## Decision (ADR-lite)

**Context:** Ungated BGE already has perfect Development final recall/current-
fact accuracy, while every false injection is an unrelated-negative turn.
Scalar/BGE intent gates and three independent candidate judges failed. Passing
all candidates to the answer model before selection would redefine injection.

**Decision:** Use the current selected GPT or DeepSeek model's exact
`search_memory` Tool decision as the next Development abstention policy. Release
the unchanged BGE Top-5 result only after a valid Tool Call. Cancel the unrun
Qwen3.5 model-only hypothesis.

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

## Expansion Sweep

- Future evolution: keep a local/private judge as an optional later profile if
  exact main-model Tool routing fails; do not add benchmark-tuned query rules.
- Related scenarios: keep L2 Scene/L3 Persona and active-reader promotion out
  of this change; they may consume a passing L1 policy later.
- Failure/edge cases: fail closed on score/model/policy drift, redaction,
  timeout, Provider failure, stale authority, and interrupted calibration.

## Out of Scope

- Promoting v2 or changing the active reader/prompt/Usage pointer.
- Enabling `MEMORY_TOOL_LOOP_ENABLED` in a deployed environment before a
  schema-v7 Development profile passes and a separate rollout decision is
  recorded.
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

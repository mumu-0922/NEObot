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

## Assumptions (updated)

- The current fixed BGE-M3 embedding/rerank models remain unchanged as retrieval
  and ordering stages.
- Fixed two-stage scores, candidate margin, bilingual BGE intent, and three
  SiliconFlow candidate-aware judges are historical failed hypotheses.
- The only active Development hypothesis is an exact `search_memory` Tool
  decision by the current user-selected GPT or DeepSeek model. Each
  Provider/model is evaluated as a separately versioned profile.
- Every hybrid relevance policy remains default-off and non-promotional until
  it passes unchanged recall, injection, latency, and authority gates.

## Open Questions

- None.

## Requirements (evolving)

### Runtime selection

- Add a versioned main-model Memory Tool-routing policy. Before a valid Tool
  Call, the Provider sees only the normal conversation context and the exact
  read-only `search_memory` definition, never Memory candidates or bodies.
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
- Require fresh, separately authorized, unexposed credentials for the fixed
  SiliconFlow BGE profile and the named GPT or DeepSeek route Provider for each
  live run. Never read or copy production-vault credentials.

## Acceptance Criteria (evolving)

- [ ] An unrelated query produces no `search_memory` Tool Call and zero hybrid
  final/injected Memory; no candidate body reaches the answer model.
- [ ] A rerank result with no score above the frozen final threshold produces
  `no_memory` and zero estimated prompt Memory tokens.
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

1. Keep `usermemory` independent from `chat` through the typed
   `HybridMemoryToolRouter` interface and fixed Tool provenance constants.
2. Own the exact no-argument `search_memory` definition in
   `internal/memoryroute`; adapt `chat.ToolPlanner` results to only zero calls or
   one non-empty-ID call with exact name and explicit `{}` arguments.
3. Extend official OpenAI and OpenAI-compatible planners with exact
   model/temperature/output bounds. Omit non-standard `enable_thinking` for
   official OpenAI and encode `false` for compatible gateways.
4. Start the redacted query-only route concurrently with fixed BGE work. Keep
   candidate content inside the separately authorized BGE boundary and release
   no hybrid final unless route model/contract provenance passes and the Tool
   call is exact.
5. Add schema-v6 Development capture, profile config v6, cost-basis v4, and a
   two-file aggregate report/manifest. Bind Provider ID/type, normalized Base
   URL SHA-256, model, Tool tuple, BGE tuple, evaluator gates, and absolute
   request/token/cost authority before Provider construction.
6. Require two independent fresh mode-`0600` credentials for live capture,
   scan both secrets from retained surfaces, and destroy all project-scoped
   Docker/runtime state on every exit path.
7. Use `fake_protocol` only for deterministic protocol/lifecycle proof. Run GPT
   and DeepSeek as separate live Development hypotheses; freeze and add a
   matching Validation lane only after one exact profile passes all unchanged
   gates.

Current checkpoint: implementation, tests, PostgreSQL 17 fake-protocol replay,
documentation, and full standalone verification pass. No real GPT/DeepSeek
Development run or schema-v6 Validation/frozen policy exists yet.

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
5. Keep v1 prompt/Usage byte-authoritative and fail closed to normal chat
   without v2 Memory on every uncertain path.

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
8. Only after one Tool-route Development profile passes, run offline/
   PostgreSQL/Compose/full gates and perform one fresh independently authorized
   validation capture without retuning.
9. Update benchmark/hybrid/Tool Loop contracts, operator workflow, and live tracking;
   retain only sanitized mode-`0600` evidence and leave promotion disabled.

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

## Expansion Sweep

- Future evolution: keep a local/private judge as an optional later profile if
  exact main-model Tool routing fails; do not add benchmark-tuned query rules.
- Related scenarios: keep L2 Scene/L3 Persona and active-reader promotion out
  of this change; they may consume a passing L1 policy later.
- Failure/edge cases: fail closed on score/model/policy drift, redaction,
  timeout, Provider failure, stale authority, and interrupted calibration.

## Out of Scope

- Promoting v2 or changing the active reader/prompt/Usage pointer.
- Injecting Memory Tool results into production before a Development profile
  passes and a separate promotion decision is recorded.
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
- No live credential exists locally now. The previous temporary Key file was
  deleted after the completed run.

# Freeze and validate the production Memory recall candidate

## Goal

Freeze the passing schema-v17 `negative guard + buffered Luna` behavior as a
new, independently versioned production candidate; prove its lifecycle with a
network-free Fake run; execute exactly one 100-case live Provider Validation;
and, only if every Validation, safety, cost, cleanup, and runtime gate passes,
automatically enable Memory recall for one exact current account while all
other accounts remain disabled.

## What I already know

- The consumed schema-v17 live Development run
  `memory-regression-20260806t082407z-1ce1eba8` passed all 300 cases and all
  unchanged quality/safety gates. It had 174 Judge attempts, nine recovered
  `PROVIDER_TRANSPORT_FAILED` attempt failures, zero terminal failures, and
  three valid abstentions.
- Its key metrics were Candidate Recall@20 `1.0`, Final Recall@5
  `0.9846153846`, current-fact accuracy `0.9818181818`, and false injection
  `0/135`. Report/manifest/configuration SHA-256 were respectively
  `d0a70c03eda7fbb1bee4107c057acc54870da56cb2041ebdb9fa4cac8955a6ce`,
  `182bbcc4cf553f9e7eb893abbd0122e9536dca970d3b232c5c7f832b703bdf2a`,
  and `83d61297ac9e0dd07a457af947642a6fb88505e2b70b701bc9e0681dd29e7359`.
- That run intentionally remained `policySelected=false` and
  `promotionEligible=false`; schema-v16 and schema-v17 evidence are consumed
  and must not be rerun or reinterpreted as production authority.
- The current product policy
  `memory_hybrid_fixed_cloud_candidate_judge_production_v1` does not enable the
  negative-policy query guard, and the current server runtime creates the
  streaming Judge adapter. Neither may be silently reused for this rollout.
- The current rollout control is process-global
  `MEMORY_TOOL_LOOP_ENABLED`; there is no per-user admission gate yet.
- The owner authorized direct online Provider testing without a quota-saving
  constraint and explicitly authorized automatic single-account rollout after
  all gates pass.

## Requirements

- Add a new production-only policy identity that preserves the schema-v17
  selection semantics and requires the frozen bilingual negative-policy query
  guard. Keep every previous policy identity byte-compatible.
- Change the product Memory Judge composition to the existing bounded buffered
  JSON adapter. Ordinary chat streaming and historical benchmark adapters must
  remain unchanged.
- Add a new immutable 100-case Validation lane with distinct capture,
  admission, profile/reader/report/run schema, artifact, policy, adapter, and
  execution-sequence identities. Do not mutate or rerun schema-v15/v16/v17.
- Reuse the established Validation corpus, criteria, BGE profile, Luna model,
  prompt, decoder, retry/cooldown behavior, failure taxonomy, privacy rules,
  and aggregate-only evidence policy. The lane must use the negative guard and
  buffered adapter as the only intended semantic/transport differences from
  the old production Validation.
- Use a new cost-basis identity while keeping ceilings large enough for the
  established 100-case/two-retry Validation contract. Testing may consume the
  authorized Provider budget, but requests must still be bounded and
  reconciled.
- Add `MEMORY_TOOL_LOOP_CANARY_USER_IDS` as an exact UUID allowlist. The global
  switch may install the infrastructure, but `search_memory` must only be
  offered when the authenticated `auth.User.ID` is in the allowlist. Empty or
  invalid configuration is fail-closed.
- Update Compose, example environment, preflight, standalone checks, runtime
  docs, and focused tests for the canary variable. Never pass it to the Memory
  Worker.
- Add a Vault-backed Validation wrapper that never builds, pulls, moves a
  mutable image tag, restarts a production service, or exposes credentials.
  Compose helper runs must use `--no-deps --no-build --pull never`.
- Prove one Fake PostgreSQL 17 lifecycle with zero network and no credential,
  database, container, network, volume, or scoped-environment residue before
  any live request.
- Execute exactly one complete live 100-case Validation. Retain aggregate
  artifacts whether the metric result passes or fails. Never automatically
  rerun a consumed live attempt.
- Auto-rollout is conjunctive: it may run only when the live report passes,
  all cases/attempts/tokens/costs reconcile, false injection/privacy/safety
  gates pass, cleanup succeeds, source quality gates pass, and pre-rollout
  runtime checks remain stable.
- On automatic rollout, select one exact existing current-login account UUID,
  pin the exact reviewed backend image, back up the live env with mode `0600`,
  render/verify the candidate environment, and recreate only `backend` with
  `--no-build --no-deps`. Do not run migrations and do not mutate Memory data.
- After rollout, verify the canary can reach the Memory Tool admission path,
  non-canary accounts cannot, backend health/logs are clean, the database
  migration version and Memory relation counts are unchanged, and unrelated
  container IDs did not change.
- If any rollout check fails, immediately restore the protected environment and
  exact previous image, recreate only `backend`, and verify health. Clearing
  the allowlist or setting `MEMORY_TOOL_LOOP_ENABLED=false` remains the instant
  reader/Judge rollback.

## Acceptance Criteria

- [x] New production policy requires the negative guard and cannot be confused
      with the old production or Development identities.
- [x] Runtime Judge uses `chat-configured-candidate-judge-buffered-v1` with
      strict authority revalidation and typed bounded failures.
- [x] Unit/race tests prove exact canary matching, empty/invalid fail-closed,
      direct-action preservation, model/tool exclusions, and non-canary zero
      retrieval/Judge work.
- [x] New Fake Validation completes exactly 100 cases with expected guard/empty/
      Judge routing and leaves no scoped or credential residue.
- [x] Existing schema-v15/v16/v17 fixtures and historical paths remain
      byte-compatible and their tests pass.
- [x] One and only one new live Validation attempt is consumed and its report,
      manifest, configuration, inputs, attempts, tokens, cost, cleanup, and
      runtime invariants are independently checked.
- [x] Failed live evidence leaves Memory recall off and preserves all data.
- [x] Passed-live auto-enable logic remains conditional and exact-user only;
      this run failed required slice gates, so the condition was not entered.
- [x] Flag-only rollout was correctly not attempted after the failed report;
      the reviewed candidate and byte-matched rollback image remain retained,
      while every live container ID stayed unchanged.
- [x] Backend tests/vet, script tests, full standalone verification, docs/spec
      consistency, and secret/security diff scan pass.

## Definition of Done

- Implementation, focused unit/integration/race tests, Fake lifecycle, all
  component gates, and full standalone verification are green.
- The sole authorized live result is retained and documented with hashes and
  decisive metrics, whether it passes or fails.
- Automatic one-account rollout is completed and verified only after a full
  pass; otherwise runtime recall remains disabled.
- Runtime secrets and raw Provider request/response bodies are never printed,
  logged, or committed.
- Work is committed only after the required one-shot commit-plan confirmation;
  no Push.

## Technical Approach

Use a successor lane rather than editing the historical Validation in place.
The production policy receives a new v2 identity and negative-guard bit; the
runtime resolver constructs the already-tested buffered Provider adapter and
wraps it with the existing transport-stable retry controller. A parsed UUID
set is injected into `chat.Handler` and checked against `auth.UserFromContext`
before any Memory Tool is exposed. The Validation successor copies the frozen
100-case machinery behind new constants and binds all report/manifest
validation to the v2 policy plus buffered adapter. The operational wrapper
separates credential export, isolated evaluation, evidence verification, and
an optional success-only canary activation guarded by immutable image and env
rollback checks.

## Decision (ADR-lite)

**Context**: schema-v17 proved the desired guard and buffered transport, but it
was Development-only. The existing production identity lacks the guard, uses
streaming at runtime, and the global flag would expose every eligible account.

**Decision**: create an isolated production-v2 candidate and schema-v18
Validation successor, then gate the product path with an exact user UUID
allowlist. Treat the user's latest instruction as final authorization for the
one-shot live run and for automatic one-account rollout after every gate passes.

**Consequences**: historical evidence stays immutable and rollback remains
simple. The implementation is broader than toggling one flag because policy,
transport, evidence identity, user admission, docs, and runtime pinning must
move together. This task does not grant an all-user release.

## Out of Scope

- Global/all-user Memory recall release, percentage rollout, role-based rollout,
  multiple-account rollout, Holdout, or post-canary general availability.
- Prompt, decoder, BGE model, Luna model, thresholds, corpus, criteria,
  retry-count, cooldown, failure taxonomy, or Memory write/extraction changes.
- Rerunning or rewriting schema-v15, schema-v16, or schema-v17 evidence.
- Database migrations, destructive data cleanup, production Memory rewrites,
  frontend redesign, or Provider credential rotation.
- Implicit image build/pull/release, mutable-tag deployment, or Push.

## Expansion Boundary

- Preserve an allowlist representation that can safely support more exact UUIDs
  later, but ship only one UUID in this canary.
- Preserve the global kill switch and evidence schemas so a future percentage
  rollout or Holdout can be introduced without weakening this gate.
- Treat Provider failure, cleanup drift, ambiguous current-account selection,
  image/schema drift, and unhealthy recreation as hard stops; none may be
  converted into a best-effort rollout.

## Technical Notes

- New identities should use a coherent schema-v18 family; exact constant names
  are implementation details but all bindings must be tested as immutable.
- Existing live schema-v17 artifact:
  `/var/tmp/neo-chat-buffered-judge-development-20260806T080832Z/live-runs/20260806T082407Z-1ce1eba8`.
- Relevant code includes `internal/usermemory/hybrid_shadow.go`,
  `internal/httpserver/server.go`, `internal/chat/memory_tool.go`,
  `internal/config/config.go`, `internal/memorycapture/`,
  `cmd/memory-regression-capture`, and `scripts/run-memory-regression.sh`.
- Runtime rollout must follow
  `.trellis/spec/operations/runtime-recreate-image-pinning.md`.

## Research References

- [`research/production-candidate-boundary.md`](research/production-candidate-boundary.md)
  — current policy/runtime/Validation gaps and the fail-closed rollout design.

## Confirmation

The owner said “开始，通过的话自动授权”. Together with the preceding explicit
single-account gray-rollout instruction, this confirms the complete scope:
execute now, use real Provider budget, and automatically authorize only the
single exact-account canary after every gate passes without asking again.

## Result

The Fake lifecycle completed all 100 cases as `35/10/55/0`
empty/guard/Judge/failed with zero network and zero residue. The sole complete
live run `memory-regression-20260806t101512z-a057b161` also completed
`35/10/55/0`, with zero terminal failures and zero false injection. Overall
Recall@20/Final Recall@5/current-fact accuracy were
`1.0/0.984615/0.981818`, but `mixed_language_entity` and `stable_fact` each had
only `0.9` current-fact accuracy and failed their required slice gates. The
immutable outcome is Yellow `retain_beta`; no canary was selected or rolled
out, both live Memory flags remain false, and the consumed run must not be
repeated.

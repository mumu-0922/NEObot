# Stabilize, validate, and launch Memory v2

## Goal

Eliminate the stochastic Luna selection defect found after schema-v20, prove a
separately versioned successor through the required Development and Validation
gates, and launch Memory v2 only after every unchanged quality, safety,
privacy, cost, cleanup, and runtime gate passes.

## What I already know

- The owner directed: pass the tests first, then launch.
- The current v1 durable-memory product path remains available and authoritative.
- Memory v2 runtime services are healthy, but
  `MEMORY_HYBRID_SHADOW_ENABLED=false`, `MEMORY_TOOL_LOOP_ENABLED=false`, and
  the exact-user canary allowlist is empty.
- The consumed schema-v20 full Development run failed two slices because Luna
  omitted the expected current fact six times.
- The separately versioned 171-execution diagnostic classified those omissions
  as stochastic across four opaque cases; no case failed all three repetitions.
- Candidate retrieval, BGE rerank, final rank/budget, and terminal-failure root
  causes were zero. Eleven transient Judge transport failures recovered.
- Schema-v20 and its diagnostic are consumed and immutable. Schema-v21 remains
  reserved for Validation and is not yet authorized or constructed.
- Existing rollout wiring is fail-closed: the product Tool path requires both a
  global flag and an exact authenticated UUID match; clearing the allowlist or
  setting the global flag false is the immediate rollback.

## Assumptions (temporary)

- The deployment currently has exactly one user. “Full launch” therefore means
  enabling the existing global Tool gate and admitting that sole authenticated
  user through the exact-UUID allowlist; it does not mean removing the
  fail-closed allowlist contract.
- No failed/consumed authority will be rerun or reinterpreted. After the
  consumed schema-v23 result, the owner explicitly accepted a separately
  versioned single-user bounded-miss criterion: overall current-fact accuracy
  remains at least `0.95`, required-slice current-fact accuracy becomes at
  least `0.90`, and false injection must be exactly zero. Historical criteria
  and reports remain immutable.
- Any real Provider Development or Validation run requires its own exact
  one-shot cost/egress authorization at the point where the frozen plan and
  upper bound are known.

## Open Questions

- The owner confirmed the prospective single-user bounded-miss criteria and
  exact-UUID rollout boundary. A new runtime defect was discovered during
  admitted smoke: migration `070` treats all current-user capture dead letters
  and scheduled `review_expire` jobs as health work. The live user has one
  ready current projection, but two historical terminal extract jobs and five
  future review-expiry jobs keep health `degraded`. Launch remains rolled back
  until migration `071` and two exact historical acknowledgements are each
  separately authorized live. No additional Provider smoke is required or
  authorized; launch must reuse the consumed smoke only as behavior evidence
  and independently re-prove health, counts, non-admission, and rollback.

## Requirements (evolving)

- Diagnose and repair Luna selection stability without changing unrelated
  Candidate, BGE, storage, v1 memory, or conversation behavior.
- Preserve the primary prompt-v2 decision. Only when its strict result is an
  empty ordinal set after Candidate/BGE admission, perform one separately
  versioned, narrowly scoped confirmation decision over the same query and
  candidates. Successful primary decisions remain one-call operations.
- Run the negative-policy guard before the confirmation path, apply the same
  strict JSON/ordinal decoder, and intersect confirmed ordinals with the fixed
  BGE set. Any malformed, failed, uncertain, or over-budget confirmation must
  fail closed to no Memory.
- Give the repair a fresh Development identity; never mutate or rerun consumed
  schema-v20 evidence and never use the reserved schema-v21 identity for
  Development.
- Run network-free Fake lifecycle and cleanup proof before any real Provider
  request.
- Run exactly one separately authorized full 300-case Development only after
  Fake and cost gates pass.
- The owner preauthorizes automatic execution of exactly one live Development
  and, only after it passes, exactly one live Validation. Before either run,
  code must derive and bind hard request, retry, input-token, output-token, and
  monetary ceilings from the frozen case plan. Missing, unbounded, drifted, or
  invalid cost authority stops before credentials or network.
- Never automatically rerun a partial, failed, consumed, or ambiguous live
  Development/Validation authority.
- Stop before Validation or launch if any unchanged global, required-slice,
  false-injection, safety, reconciliation, privacy, latency, cost, or cleanup
  gate fails.
- Construct and run schema-v21 Validation only after the new full Development
  passes every gate and the owner grants the required one-shot live authority.
- Launch only from passing immutable Validation evidence, with a recorded
  before-state, exact image/config identity, bounded smoke proof, and immediate
  rollback path.
- After every Development and Validation gate passes, automatically enable the
  reviewed Memory flags for the sole current user and execute post-launch smoke
  and rollback-readiness checks; do not pause for a second rollout preference
  decision.
- Preserve all live Memory data and leave v1 authority intact until the launch
  gate is explicitly crossed.
- Keep applied migration `070` byte-immutable. Add migration `071` so only
  `extract` jobs count as capture indexing; future `review_expire` governance
  maintenance must not affect capture health.
- Preserve every historical job, error, audit, and Activity row. A dead-letter
  extract may leave health only through one content-free, append-only,
  owner-bound acknowledgement with an exact expected error code. Require
  `SOURCE_DRIFT` plus a present non-active source Conversation for
  `source_no_longer_current`, or a terminal age of at least 24 hours for
  `historical_failure_accepted`. Runtime roles receive no table CRUD and the
  Worker receives no acknowledgement capability.
- Make `071` rollback clean only before any acknowledgement and blocked
  afterward. Applying live `071` and acknowledging each exact live job remain
  separately authorized operations; neither grants Provider or smoke replay
  authority.
- Preserve the consumed v3/schema-v21 evidence after its single overlapping
  current-fact omission. A fresh v4 Development-only successor may perform one
  additional prompt-v3 confirmation only after both the primary and first
  confirmation return valid empty selections. Stop on first selection or any
  error/provenance/malformed result; never weaken thresholds or use BGE-only
  admission.
- Preserve the consumed schema-v23 Yellow evidence after two requests remained
  valid-empty through the primary and both confirmations. Do not add a third
  confirmation or reinterpret schema-v23. Create a fresh criteria identity for
  the owner's single-user risk acceptance: keep overall current-fact accuracy
  `>=0.95`, set required-slice current-fact accuracy to `>=0.90`, require
  false-injection rate and count to equal zero, and leave every Candidate,
  Final Recall, safety, privacy, token, cost, cleanup, and runtime gate
  unchanged.
- Bind the bounded-miss criteria only to a fresh Development and fresh
  Validation identity. Both require their own Fake lifecycle, frozen cost
  authority, live approval, and immutable report; neither consumed v22 nor
  schema-v23 evidence may be rebound to the new criteria.
- Limit any resulting launch authority to the existing exact authenticated
  UUID allowlist for the sole current user. Adding another user invalidates
  the single-user risk acceptance and requires a new rollout decision.
- Let an authenticated user reject a still-pending, unexpired Review after its
  epoch, scope generation, or target Memory revision changed. Rejection may
  purge only the candidate and append the existing plaintext-free decision
  audit; `keep_current`, accept, merge, and keep-both must retain every current
  epoch/scope/target-authority fence.
- Let the sole deployment owner explicitly preview the already-generated L2
  Scene and L3 Persona readers without fabricating formal promotion evidence.
  The preview must be a separately audited schema capability, require exactly
  one current user plus ready Scene/Persona projections and zero unfinished or
  dead derived jobs, leave the formal L1 reader pointer and promotion events
  unchanged, and remain jointly gated by both existing Reader environment
  flags.
- Automatically append a fail-closed preview disable event when a second user
  is added. Deleting that user must not silently restore preview. Runtime roles
  receive neither preview activation nor event-table CRUD; rollback is an
  append-only disable event plus both Reader flags false.
- Keep the live governance UI concise: preserve L2/L3 status badges and
  controls, but omit the generic derived-content explanation below each L2
  Scene and L3 Persona profile header.
- Bound long governance lists without truncating server data: deletion progress
  and search diagnostics expose at most six cards in their scroll viewport;
  Conversation Use/Learn policy exposes at most eight cards. Each region keeps
  a stable right-side scrollbar, keyboard scrolling, and all existing actions.

## Acceptance Criteria (evolving)

- [x] Focused tests and Fake lifecycle prove the new versioned repair with zero
      network, credentials, private plaintext, or scoped runtime residue.
- [x] One authorized 300-case Development run passes all unchanged global and
      slice thresholds with exact request/token/cost reconciliation.
- [x] One authorized schema-v21 Validation run completes under unchanged gates;
      its Yellow failed-slice evidence is retained and blocks rollout.
- [x] The double-confirmation successor's failed schema-v23 Validation remains
      immutable and was superseded only through the fresh single-user bounded-
      miss criteria; neither failed schema-v21 nor schema-v23 was rerun or
      reinterpreted.
- [x] A fresh single-user bounded-miss criteria successor passes its own
      Development and Validation authorities with overall current-fact
      accuracy `>=0.95`, every required slice `>=0.90`, and exactly zero false
      injection; consumed v22/schema-v23 evidence is never rebound or rerun.
- [x] The fresh schema-v24 single-user bounded-miss live Development authority
      was consumed exactly once and passed all 300 cases, cost/reconciliation,
      safety, privacy, cleanup, and unchanged quality gates.
- [x] The fresh schema-v25 live Validation authority was consumed exactly once
      and passed all 100 cases, every required slice, exact-zero false
      injection, cost/reconciliation, safety, privacy, and cleanup gates.
- [x] No test failure, incomplete run, privacy failure, or cleanup failure can
      enable either Memory flag or populate the canary allowlist.
- [x] The separately authorized migration `070` was applied exactly once;
      subsequent rollout attempts changed only the reviewed Memory flags,
      exact-UUID allowlist, image, and required services. Persistent Memory and
      user counts remained `2/1` throughout deployment and rollback.
- [x] Post-launch smoke proves an admitted user can use the Memory Tool path and
      a non-admitted/unauthenticated user performs zero v2 retrieval/Judge work.
- [x] Rollback by clearing the canary and/or setting
      `MEMORY_TOOL_LOOP_ENABLED=false` is verified.
- [x] Offline migration `071` preserves `070`, excludes `review_expire`, and
      proves exact acknowledgement, least privilege, append-only evidence, and
      guarded `070 -> 071 -> 070 -> 071` replay on disposable PostgreSQL 17.
- [x] Live migration `071` is applied exactly once under separate authority;
      candidate services are healthy with Memory behavior still disabled and
      persistent counts unchanged.
- [x] Both exact one-job acknowledgements are applied only after their own
      authorization; health then reports `ready` without another Provider
      smoke.
- [x] Full standalone verification, docs/spec synchronization, and security
      scans pass.
- [x] A forward migration proves stale-target `reject` succeeds without
      changing the target Memory, while every target-consuming decision stays
      stale-fenced and migration down/re-up restores both behaviors exactly.
- [x] Migration `073` proves a sole-user L2/L3 Reader preview activates current
      ready artifacts without changing the L1 reader pointer or creating formal
      L2/L3 promotion events, denies runtime activation/CRUD, automatically
      disables on a second user, and cleanly replays before event history.
- [x] Live schema `073` is backed up, applied once, enabled for the exact sole
      UUID, and deployed with both Reader flags true while Backend/Worker stay
      healthy, unrelated containers stay unchanged, and formal promotion event
      counts remain zero.
- [x] The L2 Scene and L3 Persona profile headers render without the redundant
      derived-content notices in every locale, while existing controls remain.
- [x] Deletion progress and diagnostics scroll after six visible cards, and
      Conversation policy scrolls after eight, without slicing any records.

Live rollout preflight found the database at migration `069`, while the
reviewed candidate requires additive Memory-health migration `070`. Backups and
an isolated `069 -> 070 -> 069 -> 070` restore drill passed with persistent
counts unchanged. The live migration remains blocked pending explicit expanded
authority; both Memory flags and the canary remain unchanged.

The owner subsequently authorized migration `070`; it was applied exactly once
with unchanged persistent counts. Exact-UUID parser and production-policy
admission defects were repaired and verified in image
`mm-chat/backend:memory-v2-v25-exact-uuid-policy-fix-candidate-20260810t011729z`
(`sha256:c79fd467421342c63185321fa966039419528ee507c9b0538f69901fe3568e08`).
One fresh `server-default` admitted smoke completed the Memory Tool step,
released one final Memory, persisted one Usage row, and completed the assistant
message. Its one-shot authority is consumed and will not be rerun.

The smoke conversation was then soft-deleted with HTTP `204`. The mandatory
health check returned `degraded/memory_index_failed`: one current projection is
ready, while capture health contains five scheduled future `review_expire`
jobs and two historical terminal extract jobs (`EXTRACTION_INVALID` and
`SOURCE_DRIFT`). This is not a projection join/count defect. The launch gate
therefore failed and the prepared behavior rollback atomically restored both
Memory flags to false and cleared the canary, recreating only `memory-worker`
and `backend`. Both services are healthy, schema remains `070`, and persistent
counts remain `user_memories=2/users=1`. Memory v2 is not currently launched.

The owner accepted an evidence-preserving remediation. Offline migration `071`
now limits capture health to `extract`, adds a content-free append-only exact-
job acknowledgement, and denies active/missing source, wrong owner/error,
non-terminal, too-new, conflicting, direct-CRUD, and unsafe rollback paths. A
disposable PostgreSQL 17 drill passed `070 -> 071 -> 070 -> 071`; no live
migration, live acknowledgement, flag change, or Provider request occurred.
The full standalone gate then passed Frontend `964/964`, all Backend tests and
vet, and RAG `1906 passed / 7 skipped`. The packaged offline candidate is
`mm-chat/backend:memory-v2-v25-schema071-health-resolution-candidate-20260810t025553z`
(`sha256:7f54f00a930da27d5671d7f23b7ebcf9615f62198266886c151b6bc1a6a7be0b`);
it contains the migration and admin capability but has not been deployed.
On an isolated private Docker network, that exact image also applied all
migrations through `071`, cleanly downed/re-applied `071`, created and replayed
one synthetic admin acknowledgement as `true/false`, preserved the original
error, and refused down after resolution evidence.

The owner then separately authorized live migration `071`. A fresh PostgreSQL
backup passed checksum verification, the migration was applied exactly once,
and the candidate image was deployed only to `backend` and `memory-worker`.
Both services are healthy; schema/data counts are `71/2/1`, the resolution
table is empty, capture health is `0/0/2`, and projection health is `1/0/0`.
Both Memory flags remain false, the canary remains empty, and no Provider or
application Chat request occurred. The next `SOURCE_DRIFT` acknowledgement is
blocked behind its exact UUID-specific authority.

The owner then authorized both exact historical acknowledgements. The packaged
admin command created `source_no_longer_current` for the exact `SOURCE_DRIFT`
job and `historical_failure_accepted` for the exact `EXTRACTION_INVALID` job.
Both original job hashes remained unchanged; resolutions reached `2`, capture
health became `0/0/0`, and projection health remained `1/0/0`. The consumed
admitted smoke was not replayed.

A Provider-free launch preflight found no active capture or embedding work and
reconfirmed the sole UUID. The reviewed flags were atomically enabled for only
that UUID and only `memory-worker` plus `backend` were recreated on the pinned
schema-071 image. A verification-harness request to the nonexistent
`/v1/auth/me` path correctly triggered the prepared behavior rollback; after
correcting the identity path to `/v1/me`, a fresh Provider-free retry succeeded.
Final health is `ready`, both services are healthy, schema/resolution/Memory/
user counts are `71/2/2/1`, and a disposable required-auth request returned
`401` with zero Provider/Memory work. Memory v2 is now live for the sole user.

After launch, the sole pending Review became impossible to reject because its
target Memory had advanced from revision `1` to revision `2`. Forward migration
`072` now lets only the non-canonical `reject` branch bypass epoch/scope/target
currentness while preserving pending/expiry/user/replay authority. A clean
PostgreSQL 17 `071 -> 072 -> 071 -> 072` replay, focused race, all Backend
tests/vet, and the full standalone gate passed. The live retry applied `072`,
recreated only `backend` and `memory-worker`, and completed the exact rejection
with HTTP `200`, zero candidate plaintext, and no target hash/revision change.
Both services remain healthy, health is `ready`, and pending Reviews are zero.

The owner then requested active L2 Scene and L3 Persona Readers for the sole
user. Migration `073` was validated on PostgreSQL 17 and through the exact
packaged candidate over a restored schema-72 live dump before touching live.
The live migration was applied once without restarting PostgreSQL. Two
Provider-free verification-harness defects each exercised append-only preview
disable plus Reader-flag rollback before a fresh retry: the first SQL aggregate
failed because an `ORDER BY/LIMIT` scalar was not parenthesized before `UNION`;
the second incorrectly required an optional health `reason` even though the
actual response was already `ready` with `2/0/0` ready/pending/failed counts.
The final retry passed. Schema is `073`, the latest of five immutable preview
events is enabled, both derived artifacts are active and ready, the formal L1
reader pointer remains null, formal L2/L3 promotion event counts remain zero,
and Backend/Worker are healthy on the frozen schema-073 candidate. PostgreSQL
and every unrelated container kept the same ID.

## Definition of Done

- Memory v2 is launched only from passing Development and Validation evidence.
- Runtime health, data integrity, privacy, cost authority, and rollback are
  proven and recorded.
- Tests, lint, type-check, build, focused race checks, and standalone gates pass.
- Operational and benchmark documentation records exact immutable evidence.

## Out of Scope

- Retroactively weakening historical quality thresholds, selecting the best
  repetition, or rerunning/rebinding a consumed live authority. The new
  prospective bounded-miss criteria identity is explicitly in scope only for
  the sole-user rollout.
- Formal L2 Scene/L3 Persona benchmark promotion, governance UI behavior beyond
  the focused copy cleanup above, Export/Import, or Hindsight execution. The
  separately audited sole-user derived Reader preview and targeted stale-Review
  rejection above are explicitly in scope.
- Unrelated database migrations, provider changes, or broad application
  deployment.
- Multi-user rollout policy. A future additional user remains excluded until
  explicitly added to the exact allowlist.

## Technical Notes

- Governing contracts:
  `.trellis/spec/backend/memory-v2-hybrid-shadow.md` and
  `.trellis/spec/backend/memory-v2-benchmark.md`.
- Product gates are implemented by `MEMORY_HYBRID_SHADOW_ENABLED`,
  `MEMORY_TOOL_LOOP_ENABLED`, and `MEMORY_TOOL_LOOP_CANARY_USER_IDS`.
- Prior diagnostic evidence is recorded in
  `mm-chat/docs/contracts/memory-benchmark-workflow.md` and
  `mm-chat/docs/tracking/process.md`.
- See [`research/current-boundary-and-rollout.md`](research/current-boundary-and-rollout.md).
- See [`research/luna-stability-options.md`](research/luna-stability-options.md).
- See
  [`research/live-migration-071-and-health-resolution-plan.md`](research/live-migration-071-and-health-resolution-plan.md)
  for the separately gated no-Provider live continuation.
- See [`research/live-migration-071-result.md`](research/live-migration-071-result.md).
- See
  [`research/live-schema072-stale-review-reject-result.md`](research/live-schema072-stale-review-reject-result.md).
- See
  [`research/live-schema073-single-user-derived-reader-preview-result.md`](research/live-schema073-single-user-derived-reader-preview-result.md).

## Decision (ADR-lite)

**Context**: The deployment is single-user, while the existing Memory v2
product contract requires a global gate plus an exact authenticated UUID
allowlist.

**Decision**: Treat exact admission of the sole current user as the full
production launch. Preserve the allowlist and fail-closed checks rather than
adding an unrestricted all-user bypass.

**Consequences**: The owner receives Memory v2 immediately after all gates pass
without broadening the security contract. Clearing the one UUID or disabling
the global flag remains an immediate rollback.

### Luna stability decision

**Context**: Prompt v2 already carries an explicit no-abstention instruction,
but the v20 diagnostic still observed stochastic empty selections. Bypassing
Luna with a BGE-only fallback would weaken the semantic safety boundary.

**Decision**: Add exactly one separately versioned confirmation decision only
after an otherwise valid empty Luna result. Do not alter Candidate retrieval,
BGE, negative guard, primary prompt v2, strict decoding, final intersection, or
quality thresholds.

**Consequences**: Ordinary successful requests retain current cost and latency;
abstentions may incur one extra bounded Judge decision. Development and
Validation must prove false injection remains zero and bind the increased
request/token ceiling before any paid run.

### Paid-run decision

**Context**: Semantic quality can be proven only with real fixed BGE/Luna
Provider calls after network-free Fake lifecycle proof.

**Decision**: The owner preauthorizes automatic execution of one bounded live
300-case Development and, only if it passes, one bounded schema-v21 Validation.
The exact cost documents and all hard ceilings must validate before credential
export or network. There is no automatic rerun authority.

**Consequences**: The workflow can proceed to launch without pausing at the
paid gates, but any ceiling drift, incomplete evidence, failed metric, cleanup
failure, or runtime drift stops the chain with Memory v2 still disabled.

### Single-user bounded-miss decision

**Context**: Schema-v23 completed all 100 Validation cases with zero false
injection and zero safety/privacy failure, but two valid-empty requests caused
three required 10-case slice memberships to score `0.9`. A `0.95` threshold on
ten current-fact cases is discretely equivalent to requiring `10/10`. The
owner accepts occasional fail-closed omission for the sole current user but
does not accept false Memory.

**Decision**: Add a prospective, separately versioned criteria profile. Keep
overall current-fact accuracy at `0.95`, set required-slice current-fact
accuracy to `0.90`, and strengthen false injection to an exact-zero gate. Keep
the v4 double-confirmation reader unchanged. Require fresh Development and
Validation evidence before exact-UUID rollout.

**Consequences**: Schema-v23 remains failed and cannot launch. The new lane may
accept at most one current-fact miss in a ten-case required slice while still
rejecting any false Memory, safety/privacy failure, global-quality failure,
terminal failure, cost drift, or cleanup residue. The acceptance applies only
to the current single-user deployment.

## Expansion Boundary

- **Future evolution**: preserve the exact-user allowlist so later users can be
  admitted deliberately; do not add an unrestricted global audience path now.
- **Related behavior**: v1 memory remains the rollback authority; conversation,
  storage, Usage, and non-Memory Provider flows must remain unchanged.
- **Failure handling**: no automatic live rerun; no launch after partial or
  ambiguous evidence; flags and sole-user admission change only after immutable
  Validation pass and pre-deployment state capture.

## Technical Approach

1. Add a separately versioned confirmation prompt/adapter boundary and policy
   identity. Invoke it only after a valid empty primary prompt-v2 result.
2. Extend aggregate-only telemetry and cost reconciliation to distinguish the
   primary decision, semantic confirmation, and transport retries without
   persisting query, Memory, scores, Provider bodies, or raw error text.
3. Add a fresh full 300-case Development profile/report/run/cost/wrapper
   identity, network-free Fake fixtures, lifecycle cleanup, and no-rerun guard.
4. If Development passes every unchanged criterion, add the reserved
   schema-v21 100-case Validation identity over the exact selected policy, with
   separate cost authority, Fake proof, and one-shot live wrapper.
5. Promote only the validated policy into the existing production composition,
   build and pin the reviewed image, capture live pre-state, enable the required
   flags for the sole authenticated user, recreate only required services, and
   prove admitted/non-admitted behavior plus rollback readiness.
6. Run focused race checks, full backend checks, complete standalone
   verification, documentation/spec synchronization, and secret/security scans.
7. After the consumed schema-v23 Yellow result, add a fresh bounded-miss
   criteria evaluator and independently versioned Development/Validation
   authorities without changing the v4 reader. Prove historical criteria JSON
   and hashes remain byte-stable, then repeat Fake, cost, live, and rollout
   gates under the new prospective standard.
8. After L2/L3 shadow artifacts are current and ready, add migration `073` as a
   sole-user preview authority rather than weakening or forging the formal
   promotion lane. Prove PostgreSQL replay/privilege/population-change behavior,
   then apply it from a fresh backup and recreate only Backend/Worker with both
   Reader flags enabled on the frozen candidate image.

## Implementation Plan

- **Slice 1**: confirmation policy, strict adapter behavior, telemetry, unit and
  race tests.
- **Slice 2**: versioned 300-case Development authority, Fake lifecycle, cost
  bounds, Vault wrapper, documentation.
- **Slice 3**: execute the sole live Development; stop on any failed gate.
- **Slice 4**: schema-v21 Validation authority and Fake lifecycle, then execute
  the sole live Validation if Development passed.
- **Slice 5**: production policy promotion, pinned deployment, sole-user launch,
  smoke/rollback proof, full verification and records.

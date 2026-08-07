# Diagnose Memory validation slice failures

## Goal

Create an independently versioned, diagnostic-only successor that explains the
single current-fact miss behind the failed schema-v18
`mixed_language_entity` and `stable_fact` slice gates, then use that evidence
to make at most one minimal fix and prove it on a fresh full Development lane
before any new Validation or runtime rollout is considered.

## What I already know

- The sole schema-v18 live Validation is consumed and cannot be rerun.
- Its Candidate Recall@20 was `1.0`, while Final Recall@5/current-fact accuracy
  were `64/65` and `54/55` respectively.
- Exactly one current-fact miss reduced both failed slices from `10/10` to
  `9/10`; the protected Validation corpus has exactly two cases in that slice
  intersection, but aggregate-only evidence intentionally does not identify
  the failed case.
- The protected Development split offers 85 non-Holdout cases across the two
  failed slices, including five intersection cases.
- Current process-local traces cannot separate BGE reranked IDs from
  Luna-selected IDs, so another aggregate-only run cannot identify the layer.
- The owner previously authorized direct online Provider testing without a
  quota-saving constraint. Production Memory recall remains disabled and all
  data must be preserved.

## Confirmed Decisions

- Use one schema-v19 live attempt containing three fixed repetitions of the
  85-case Development union (`255` executions) to distinguish systematic from
  stochastic failure.
- Retain only opaque case/stage identities in a protected mode-`0600`
  diagnostic artifact; retain no plaintext or raw Provider bodies.
- After a decisive diagnosis, automatically apply at most one minimal,
  separately versioned fix and run a full 300-case Development successor.
- If that full Development successor passes every unchanged gate, continue to
  a fresh 100-case Validation successor with new identities and exactly one
  live attempt. A complete Validation pass may automatically enable the
  existing exact-account canary; any failure keeps recall disabled.

## Requirements (evolving)

- Preserve schema-v15 through schema-v18 code, identities, artifacts, reports,
  and consumed-run authority byte-for-byte.
- Never rerun the schema-v18 Validation cases or inspect Holdout.
- Add a diagnostic-only Development lane with new capture, profile, reader,
  report, manifest, artifact, cost, approval, and execution identities.
- Select exactly the 85 Development cases in the union of
  `mixed_language_entity` and `stable_fact`, with a hash-bound order, and run
  exactly three repetitions in one bounded attempt.
- Capture separate opaque Candidate, BGE rerank, Luna selection, intersection,
  and Golden-current membership outcomes per case/repetition.
- Retain no query/Memory plaintext, embeddings, scores beyond bounded stage
  classification, Provider bodies, raw errors, credentials, or URLs.
- Publish a private mode-`0600` diagnostic artifact plus an aggregate manifest;
  both remain non-promotional and cannot mutate runtime flags.
- Reuse the frozen production-v2 BGE/Luna model, prompt, decoder, guard,
  provider binding, retry/cooldown, and failure taxonomy so the diagnostic
  changes observability and Development selection only.
- Add a Vault-backed wrapper with capability-detected Compose `run --no-build`,
  mandatory `--pull never`, a pinned candidate image, exact cleanup, and no
  production service recreation.
- Complete a network-free Fake PostgreSQL 17 lifecycle before any live request.
- Treat any credential, cleanup, cost, count, authority, or privacy drift as a
  hard stop; never automatically retry a consumed complete live diagnostic.
- Keep `MEMORY_HYBRID_SHADOW_ENABLED=false`,
  `MEMORY_TOOL_LOOP_ENABLED=false`, and the canary allowlist empty throughout
  diagnosis and Development.
- After a decisive schema-v19 result, change exactly one responsible semantic
  boundary—BGE selection, Luna prompt/policy, or final intersection—and assign
  fresh schema-v20 Development identities. Do not bundle unrelated tuning.
- Run the complete 300-case Development split online after that minimal fix,
  retaining only aggregate evidence under unchanged quality, safety, privacy,
  retry, and cost-reconciliation gates.
- Only a complete schema-v20 Development pass may construct schema-v21
  Validation. Prove its Fake PostgreSQL 17 lifecycle before exactly one live
  100-case Validation attempt with fresh approval/cost/artifact identities.
- Only a complete schema-v21 Validation pass plus source/runtime/cleanup gates
  may select one exact current-login account UUID and recreate only the pinned
  backend. Any failure leaves both Memory switches false and the canary empty.
- Reuse the existing Vault-backed BGE and Luna credentials; never print, log,
  commit, or broaden their export surface.

## Acceptance Criteria (evolving)

- [x] Static tests prove exactly 85 selected Development cases, five slice-
      intersection cases, three repetitions, stable order/hash, and zero
      Validation/Holdout admission.
- [x] Tests prove Candidate/BGE/Luna/Final stage classification identifies the
      first layer that lost the expected current fact without retaining
      plaintext or raw Provider output.
- [x] Fake completes 255 case executions with zero network and zero credential,
      database, container, network, volume, or scoped-environment residue.
- [x] Exactly one complete live schema-v19 diagnostic is consumed and all
      cases, attempts, retries, tokens, costs, permissions, and cleanup
      reconcile.
- [x] The diagnostic result classifies the failure as systematic or stochastic
      and identifies Candidate, BGE, Luna, or final-intersection responsibility.
- [x] Exactly one minimal versioned fix is applied to the diagnosed stage, and
      a fresh 300-case Development successor completes with reconciled online
      metrics and immutable aggregate evidence.
- [x] A failed Development result stops before Validation. A passed Development
      result enters one fresh Fake plus one fresh live 100-case Validation, with
      no whole-run retry.
- [x] Because schema-v20 Development failed, schema-v21 Validation and canary
      construction were not entered; recall remains off and all Memory data is
      preserved.
- [x] Schema-v15 through schema-v18 fixtures and artifact hashes remain
      unchanged.
- [x] Runtime Memory flags, canary configuration, database migration version,
      Memory relation counts, and live container IDs remain unchanged.
- [x] Focused race tests, all backend tests/vet, script lifecycle tests,
      standalone full verification, docs/spec sync, and secret/security scans
      pass.

## Definition of Done

- Diagnostic identities, implementation, focused tests, Fake lifecycle, one
  bounded live diagnostic, and cleanup evidence are complete.
- A decisive root-cause report is retained without private plaintext.
- One minimal versioned fix and one fresh 300-case Development successor are
  implemented and evaluated automatically after decisive diagnosis.
- A passing Development successor automatically enters one new Validation
  successor; a passing Validation automatically rolls out only the existing
  exact-account canary. No migration, destructive Memory mutation, global
  release, or Push is authorized.

## Technical Approach

Extend the process-local Recorder/capture trace with distinct opaque stage
snapshots, then build a schema-v19 report over a deterministic Development-only
slice-union plan. Use three repetitions inside one live authority so the report
can distinguish stable stage loss from nondeterministic Judge behavior. Keep
the schema-v18 aggregate path untouched. Apply one evidence-selected change and
bind it to schema-v20 full Development identities. Only a full pass creates a
schema-v21 Validation successor; only a complete Validation pass enters the
already-implemented exact-UUID canary rollout and rollback path.

## Decision (ADR-lite)

**Context**: v18 proves one post-Candidate current-fact miss but deliberately
omits the case and intermediate stage identities needed for diagnosis.

**Decision**: introduce a private, opaque, Development-only stage diagnostic
rather than rerunning Validation, weakening slice gates, or changing the model
blindly.

**Consequences**: the task gains enough observability to isolate BGE versus
Luna without exposing synthetic text or granting promotion authority. It costs
up to 255 focused case executions before any full Development proof. The
automatic chain remains fail-closed at every boundary: diagnostic cannot grant
promotion, Development cannot enable runtime, and only a fresh Validation pass
can admit the single canary.

## Out of Scope

- Rerunning/reinterpreting schema-v18, inspecting Holdout, weakening `0.95`
  current-fact criteria, or selecting the better of repeated Validation runs.
- Global/all-user Memory rollout, percentage rollout, migrations, destructive
  cleanup, Memory writes, or frontend changes. Exact one-account canary rollout
  remains in scope only after the fresh Validation successor passes.
- Changing BGE/Luna providers, model IDs, credentials, corpus, Golden labels,
  retry policy, or safety/privacy criteria merely to obtain a pass.
- Committing private case-level artifacts, credentials, Provider bodies, or
  protected corpus content.

## Expansion Boundary

- Preserve a reusable stage-classification type for future slice diagnostics,
  but bind schema-v19 to only these two failed slices and this fixed plan.
- A future new Validation must use fresh case-order/report/cost/approval
  identities and may begin only after a full Development successor passes.
- Preserve the stage-classification seam for later diagnostics, but do not turn
  schema-v19 case-level evidence into a general production logging feature.

## Technical Notes

- Retained v18 aggregate evidence:
  `/var/tmp/neo-chat-production-buffered-validation-20260806T095727Z/live-runs/20260806T101512Z-a057b161`.
- Relevant code: `internal/memorycapture/{types,capture,recorder}.go`,
  `production_buffered_memory_judge_validation.go`,
  `cmd/memory-regression-capture`, and `scripts/run-memory-regression.sh`.
- Runtime helper-image rules remain governed by
  `.trellis/spec/operations/runtime-recreate-image-pinning.md`.

## Research References

- [`research/schema-v19-diagnostic-boundary.md`](research/schema-v19-diagnostic-boundary.md)
  — evidence narrowing, current observability gap, fixed Development subset,
  and recommended three-repetition diagnostic plan.

## Confirmation

The owner replied “开始” to the proposed chain of schema-v19 diagnosis,
minimal repair, full 300-case Development, fresh Validation, and automatic
single-account canary after a complete pass. The owner then chose option `A`,
explicitly authorizing automatic minimal repair plus the full Development run.
Earlier authorization permits direct online Provider testing without a
quota-saving constraint; all requests remain bounded and reconciled.

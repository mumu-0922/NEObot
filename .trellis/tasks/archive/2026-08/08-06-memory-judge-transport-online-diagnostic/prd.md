# Diagnose Memory Judge transport online

## Goal

Replace only the fixed Luna Memory Judge's SSE response transport with a
bounded non-streaming JSON completion path, give the change an isolated
schema-v17 full-Development identity, and obtain one complete online result
without changing Memory relevance behavior or production authority.

## What I already know

- The consumed schema-v16 live run completed 300 cases but recorded 23 typed
  Judge attempt failures and three terminal transport failures.
- It also recorded five valid Judge abstentions, so eliminating transport
  failures may not be sufficient to pass the two current-fact slices.
- Schema-v16 is immutable and must not be rerun.
- Memory recall and Tool Loop remain disabled, and the user authorized direct
  online Provider testing without a quota-saving constraint.
- The OpenAI-compatible Provider already owns a non-streaming JSON code path;
  Memory Judge wire handling does not need to be duplicated outside
  `internal/chat`.

## Assumptions (temporary)

- Use the retained schema-v16 report as the streaming baseline rather than
  spending requests on a second streaming arm.
- Preserve two Judge retries and the five/ten-second fallback schedule. A retry
  increase would be a second experimental variable.
- Use reader v15, profile/report v17, cost-basis v12, and a distinct capture and
  admission mode.
- Preserve failure taxonomy v1 by mapping buffered body-read interruption to
  retryable `PROVIDER_TRANSPORT_FAILED`.

## Requirements (evolving)

- Add an optional bounded completion interface owned by `internal/chat`; do not
  change ordinary chat streaming.
- Add a separately versioned Memory Judge buffered adapter that reuses the
  exact existing prompt, decoding, model, temperature, no-thinking, and token
  controls.
- Reject missing content, multiple choices, incomplete, malformed, oversized,
  late, or typed Provider-failure responses without persisting bodies.
- Add a distinct schema-v17 Development capture/profile/reader/report identity,
  cost-basis v12, artifact name, manifest admission mode, and Vault wrapper.
- Keep the schema-v16 guard policy/version/SHA/descriptor and all criteria,
  corpus, BGE, retry, cooldown, cost, privacy, and cleanup behavior unchanged.
- Prove Fake PostgreSQL 17 lifecycle before exactly one complete live run.
- Retain aggregate artifacts on passed or failed metric gates; never promote,
  start Validation/Holdout, mutate production policy/data, enable recall, or
  rerun automatically.

## Acceptance Criteria (evolving)

- [x] Streaming and buffered requests are wire-equivalent except for the
      transport fields and response framing.
- [x] Historical adapters/schemas remain byte-compatible and their tests pass.
- [x] Buffered malformed/read/interruption/status/cancellation paths produce
      bounded typed failures with exact retry behavior.
- [x] New Fake lifecycle completes all 300 cases with zero network and zero
      scoped or credential residue.
- [x] One full live run reconciles all 300 outcomes, attempts, tokens, cost,
      guard counts, safety counters, and cleanup.
- [x] Runtime Memory flags and sampled live Memory state remain unchanged.

## Definition of Done

- Focused race tests, all backend tests/vet, script lifecycle tests, and full
  standalone verification pass.
- The authorized live result is independently checked and documented whether
  it passes or fails.
- Specs and operational docs record the transport-only identity and outcome.
- Work is committed only after an explicit commit-plan confirmation; no Push.

## Out of Scope

- Prompt, decoder, threshold, guard, relevance policy, BGE, retry-count, corpus,
  criteria, or Provider/model changes.
- Schema-v16 rerun, dual-arm streaming probe, Validation, Holdout, promotion,
  Release, deployment, or Memory recall re-enable.
- Treating a transport-clean result as permission to ignore remaining quality
  failures.

## Technical Notes

- Recommended capture mode:
  `development_fixed_memory_judge_negative_guard_buffered`.
- Recommended admission mode:
  `development_fixed_memory_judge_negative_guard_buffered_only`.
- Reuse `neo-chat.memory-regression-relevance-run.v1` for the aggregate
  manifest, but bind the new capture/admission/adapter identities.
- Keep v12-equivalent request ceilings: 900 Judge requests, 1,500,000 input
  tokens, and 115,200 output tokens. Only the cost schema identity changes.
- Live credentials and Provider request/response content must never be logged,
  printed, or persisted.

## Research References

- [`research/transport-mode-design.md`](research/transport-mode-design.md) —
  runtime evidence, current code ownership, invariants, and approach analysis.

## Decision (ADR-lite)

**Context**: Schema-v16 showed that the fixed Luna SSE path can fail after two
retries, but rerunning the same evidence or changing relevance behavior would
confound the next result.

**Decision**: Use Approach A: add a Provider-owned buffered JSON completion and
a schema-v17 full-Development successor, with schema-v16 as the historical
streaming baseline.

**Consequences**: The result isolates response framing under representative
load and may prove transport stability. It can still fail unchanged positive
quality gates and remains non-promotional.

**Confirmation**: The owner selected Approach A and authorized the direct
online full-Development run under the standing no-quota-saving instruction.

## Result

- Fake run `memory-regression-20260806t081327z-a45126d9` completed
  `105` empty-candidate + `30` guard + `165` Judge cases and passed with zero
  network, credential, or scoped Compose residue. Report/manifest SHA-256:
  `7d9dfc5edd93a8c331b92230f6f42719ae06c1402f43f056ce7530ea17cad113` /
  `8d5b8bc3c68997b5a6c2141376303bd9b04ecc3f67406750a5d6c0b5bb82119e`.
- The only live run, `memory-regression-20260806t082407z-1ce1eba8`, completed
  all 300 cases with `174` Judge attempts, nine recovered
  `PROVIDER_TRANSPORT_FAILED` attempt failures, zero terminal failures, and
  three valid abstentions. Candidate Recall@20 was `1.0`, Final Recall@5
  `0.9846153846`, current-fact accuracy `0.9818181818`, and false injection
  `0/135`; every slice, safety, privacy, token, and cost gate passed.
- Live report/manifest/configuration SHA-256:
  `d0a70c03eda7fbb1bee4107c057acc54870da56cb2041ebdb9fa4cac8955a6ce` /
  `182bbcc4cf553f9e7eb893abbd0122e9536dca970d3b232c5c7f832b703bdf2a` /
  `83d61297ac9e0dd07a457af947642a6fb88505e2b70b701bc9e0681dd29e7359`.
  The raw/decoded v12 cost hashes were
  `a8e339b0aff182773b886681ad125eb5dcc6205d705cf325309c698da9b44d6a` /
  `339d419caa56ba7414ec993b2d059f004279315de65e05479b603536cbeb17f4`.
- Pre/post counts across 43 production Memory relations remained byte-equal at
  aggregate SHA-256
  `d027b35dd8b667f21c84b2a38cd0b27fec94b684c0d4561c8677bb3b9885142b`;
  both Memory runtime flags stayed false. The result remains
  `policySelected=false` and `promotionEligible=false`.
- Final verification passed: focused race, all backend tests, `go vet ./...`,
  Memory regression and Vault lifecycle scripts, diff secret/security scan,
  and `bash scripts/verify-standalone.sh --full` (frontend `964` tests; RAG
  `1906` passed / `7` skipped).
- During review, the Vault wrapper's implicit image-tag mutation risk was
  removed. Credential export now uses Compose `--no-build --pull never`; the
  running production services were not restarted or changed.
- Schema-v16 and schema-v17 live authority are consumed. Do not rerun either,
  start Validation/Holdout, enable recall, promote, Release, deploy, or Push
  from this evidence.

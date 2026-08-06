# Analyze and remediate Memory false injection

## Goal

Explain the schema-v15 Orange false-injection result from immutable evidence
and introduce a deterministic, versioned Development-only guard for negative
Memory meta-policy queries without changing production policy, runtime flags,
or persistent Memory data.

## What I already know

- The one authorized Validation completed all 100 cases but produced 9 false
  injections, rate `0.09`, above the frozen `0.02` ceiling.
- All nine are contained within the 10-case `unrelated_negative` slice.
- Exact failed case IDs cannot be recovered because the retained report is
  aggregate-only.
- A narrow offline bilingual prototype covers all 10 possible failed cases and
  6 additional negative cases while matching zero relevant Validation cases.
- Memory recall and hybrid Provider work remain disabled in the live stack.

## Requirements

- Preserve the immutable report/manifest and verify their hashes before using
  them as evidence.
- Do not inspect or evaluate Holdout content.
- Do not invoke BGE, rerank, Luna, DeepSeek, or any other Provider.
- Add a bilingual negative meta-policy guard with a frozen version and SHA-256.
- Require the guard only in a new Development policy identity; do not mutate or
  alias the current production-v1 policy.
- Apply the guard after authorized repository preparation but before candidate
  admission, rerank, and Judge egress.
- Record a completed empty final set with bounded fallback
  `NEGATIVE_POLICY_QUERY_ABSTAINED` when the guard matches.
- Preserve old policy-descriptor JSON/hashes by omitting new descriptor fields
  when the guard is disabled.
- Add focused tests proving:
  - bilingual meta-policy negatives abstain;
  - ordinary relevant personal-memory requests do not match;
  - guarded execution performs no admission, rerank, or Judge call;
  - the recorded final set is empty and fallback is exact;
  - production-v1 descriptor/hash and behavior remain unchanged.
- Re-run the consumed-Validation-only offline classification and bind its case
  set/counts in a non-provider diagnostic result.

## Acceptance Criteria

- [x] Evidence hashes match the immutable live-run report and manifest.
- [x] Exact-nine attribution is explicitly rejected as unavailable evidence.
- [x] Guard flags all 10 `unrelated_negative` Validation cases and zero of 55
      relevant Validation cases in the offline audit.
- [x] New Development policy is valid, accuracy-first, hash-bound, and not
      installed by the Server composition root.
- [x] Guarded requests release zero Memory and cause zero candidate Provider
      egress.
- [x] Existing production policy descriptor hash remains
      `c65c2b0bee2561ebbc8d97a65c4cc0c64db243b8a09334a8f1836250d799095c`.
- [x] Focused Go tests, backend vet/tests, and full standalone gate pass.
- [x] Runtime flags remain false and live Memory row counts are not mutated.

## Definition of Done

- Root cause and evidence limits are documented.
- The deterministic guard exists behind a Development-only policy and has
  focused regression coverage.
- Specs/docs describe the new boundary and the required future calibration
  sequence.
- No Development live run, Validation, Holdout, promotion, release, re-enable,
  migration, or Push occurs.

## Technical Approach

Extend `HybridShadowRelevancePolicy` with an opt-in negative-policy-query guard
and add `omitempty` descriptor fields so every historical descriptor remains
byte-identical. Add a new accuracy-first Development mode/ID. In
`executeHybridShadow`, evaluate the guard after `PrepareHybridShadow` and
before admission/rerank/Judge; record an empty result through the existing
observation path. Keep the current production policy unchanged.

## Decision (ADR-lite)

**Context**: Luna Judge selected Memory for 9/10 adversarial policy questions,
while aggregate evidence retained no safe score threshold or exact case-level
outputs.

**Decision**: Prefer a deterministic query-shape guard over prompt-only or
numeric-threshold changes, and change only that variable in this cycle.

**Confirmation**: The owner explicitly selected Approach A.

**Consequences**: The known failure class becomes fail-closed and auditable,
but the new policy cannot be promoted until a separately authorized
Development calibration and subsequent new Validation pass.

## Out of Scope

- Naming the exact nine cases without evidence.
- Modifying the frozen corpus, criteria, or historical report.
- Inspecting or running Holdout.
- Calling any live Provider.
- Changing production policy/flags, re-enabling recall, migrating, releasing,
  or pushing commits.

## Research References

- [`research/false-injection-analysis.md`](research/false-injection-analysis.md)
  — immutable evidence correlation, offline prototype, and approach comparison.
- [`research/negative-policy-guard-validation-audit.json`](research/negative-policy-guard-validation-audit.json)
  — provider-free consumed-Validation counts, hashes, and evidence boundary.

## Verification

- `go test -race ./internal/usermemory ./internal/memorycapture` passed.
- `go vet ./...` and `go test ./...` passed.
- `bash mm-chat/scripts/verify-standalone.sh --full` passed, including 964
  frontend tests, all backend packages, and 1906 passed/7 skipped RAG tests.
- The live report, manifest, and corpus SHA-256 values matched their frozen
  evidence bindings.
- Runtime flags remain false; all 43 sampled Memory relations match the
  protected post-disable snapshot with zero additions, removals, decreases, or
  count changes.
- No Provider call, Development/Validation/Holdout run, deployment, migration,
  promotion, Release, recall re-enable, or Push occurred.

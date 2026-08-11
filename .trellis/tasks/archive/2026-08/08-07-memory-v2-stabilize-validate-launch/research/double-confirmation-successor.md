# Double-confirmation successor boundary

## Evidence

Schema-v21 completed 100/100 cases with zero false injection and zero safety
failures, but one current-fact omission failed both overlapping
`mixed_language_entity` and `stable_fact` slices. That case reached the valid-
empty primary path and consumed one valid-empty confirmation. The earlier
three-repetition v20 diagnostic found no opaque case that abstained in all
three independent decisions.

## Decision

Create a fresh v4 Development-only policy that preserves prompt v2, prompt v3,
negative guard, BGE admission/intersection, strict decoder, retry behavior,
token budget, and all quality thresholds. It may make at most two logical
confirmation decisions:

1. Run primary prompt v2.
2. If and only if it returns a valid empty selection, run confirmation v3.
3. If and only if that confirmation also returns a valid empty selection, run
   the same confirmation v3 once more.
4. Stop at the first non-empty valid result. Any malformed output, Provider
   failure, provenance drift, or context cancellation fails closed immediately
   and never advances to another confirmation.

The consumed v3 Development and schema-v21 production-policy identities remain
byte-immutable. The v4 descriptor adds an explicit maximum-confirmation count
only for the new policy; v3 descriptors omit the field so their hashes remain
unchanged. Production remains unreachable until fresh Development and
Validation authorities pass.

## Implemented core proof

- New development-only policy ID:
  `memory_hybrid_fixed_cloud_candidate_judge_negative_guard_double_confirmation_development_v4`.
- The v3 Development and production policies explicitly remain bounded to one
  confirmation.
- Unit coverage proves first-confirmation short circuit, second-confirmation
  rescue, two-empty termination, and error fail-closed without a third call.
- The generic aggregate telemetry validator now receives an explicit maximum
  logical-confirmation count. Consumed v3/schema-v21 specs pass `1`; the v4
  execution policy passes `2`. A regression proves two logical confirmations
  are accepted only under the v4 bound and rejected under v3.
- Fresh v22 Development reader/report/run/admission/artifact and cost schema
  constants are defined. The report spec binds policy v4, two confirmations,
  and a `2700`-attempt ceiling. V3 and v22 cost documents reject each other.
- The v22 run-manifest builder now validates the v4 report, execution policy,
  artifact name/hash, cost hash, protected input hashes, and exact new capture
  mode before publishing its independent completion marker.
- `go test ./...` and `go vet ./...` pass with production composition unchanged.

## Full-path Fake result and frozen Development cost

The first real Docker Fake attempt exposed a capture-only state-machine defect:
the Recorder finalized the first valid-empty confirmation and rejected the
second egress. The production reader loop was already correct. The Recorder now
tracks the primary result plus bounded confirmation egress/input/result counts;
the v3 decorator fixes its maximum at one, while the v4 decorator fixes it at
two. It finalizes only on the first non-empty result or the last permitted empty
result, and rejects all later work after a result, malformed response, Provider
failure, provenance drift, or cancellation.

A fresh PostgreSQL 17 Fake run then completed all `300` Development cases and
passed every aggregate quality and safety gate. It forced `165` primary Judge
decisions plus exactly `330` confirmation decisions, for `495` attempts with no
retry. Judge input-token upper bounds reconciled at `886206` total and `590144`
confirmation-only. All artifacts were mode `0600`, and the scoped containers,
network, and volume were destroyed.

The maximum retry contract permits three attempts for every logical Judge
decision. Therefore the deterministic full-path input bound is
`886206 * 3 = 2658618`, rounded upward to the frozen `2700000` hard ceiling.
The existing `2700` request and `345600` output-token ceilings remain unchanged;
at the fixture rates the resulting maximum Judge cost is
`3045600000000` owner-budget microunits. The earlier `3000000` document remains
only with the immutable Fake lifecycle evidence and is not live authority.
The final document's raw and canonical decoded SHA-256 values are
`82771c9ebb4dd521cd4b5aa584a58f39cc2e786b3677ced6e523755c8a4c2de0`
and `1599e276ae4de940a889b51b5b11e861483b25558b9be34b66607f97717bce16`.

## Current authority boundary

- The one-shot v22 live Development authority was consumed once and passed.
  It cannot be rerun, even after a later Development authorization.
- Schema-v23 now provides the separately versioned production-policy
  Validation identity. Its PostgreSQL 17 Fake lifecycle and final cost
  authority are frozen in `double-confirmation-validation-preflight.md`.
- The independent Vault and live-shaped topology gates pass. No schema-v23
  Provider request may run until the owner grants one fresh exact live
  double-confirmation Validation authority.

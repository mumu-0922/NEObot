# Live Development pre-network failure

## Result

The single authorized abstention-confirmation Development invocation started
at `2026-08-07T09:47:14Z` and failed before any BGE or Luna request. No report
or manifest was published. The wrapper destroyed both exported credential
copies and the isolated PostgreSQL/Compose runtime.

## Root cause

`scripts/run-memory-regression.sh` had three related mode predicates:

1. validate and normalize the configured Judge tuple;
2. preflight and copy the host Judge credential;
3. set the in-container Judge credential target.

The new Development and Validation modes were present in (1) and (3), but
absent from the preflight/copy predicates in (2). The runner therefore created
and mounted an empty `configured-candidate-judge-provider.key`; command startup
failed with `read configured candidate-judge credential failed`.

## Repair and prevention

- Add both confirmation modes to the host credential preflight and copy
  predicates.
- Execute live-shaped Development and Validation cases in
  `scripts/test-memory-regression.sh`.
- Make the fake runner assert the copied Luna credential bytes and exact
  in-container target for both modes.
- Keep the consumed attempt marker and do not automatically rerun the live
  authority.

## Rollout state

Live Validation and launch were not attempted. Memory flags remain false, the
canary allowlist remains empty, and v1 remains active.

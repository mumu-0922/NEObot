# Production promotion evidence design

## Recommended shape

Use one strict JSON closure record with:

- an explicit `template` versus `production` evidence class;
- exact release commit, migration head, Runner release manifest, Runtime
  Bundle and operations-policy SHA-256 bindings;
- a closed set of required checks, each with result, observed time and evidence
  SHA-256 rather than raw log or workload content;
- cleanup zero-counts for temporary canary Runs, Drafts, Artifacts and Sandbox
  residue;
- explicit review time and promotion window.

The gate derives `PROMOTION_READY`, `PROMOTION_HELD` or
`PROMOTION_EVIDENCE_INVALID`; the input cannot self-assert promotion state.

## Required proof classes

1. exact-host isolation and full negative suite;
2. clean-copy install and immutable release binding;
3. restart plus host-reboot reconciliation;
4. paired backup/restore and disaster-recovery rehearsal;
5. rollback and forward-fix rehearsal;
6. hierarchical Kill Switch exercise;
7. Runner mTLS/credential and Runtime Bundle rotation;
8. orphan/descendant/Scratch reconciliation;
9. `outcome_unknown` no-retry operator workflow;
10. metrics scrape, alert routing and capacity/budget enforcement;
11. bounded synthetic/read-only canary;
12. temporary evidence cleanup with the sanitized promotion record retained.

## Fail-closed rules

- `template` evidence never promotes even if edited to say every check passed.
- A non-`passed` check holds promotion. `isolation_unavailable` maps to the
  stable held reason `ISOLATION_UNAVAILABLE`.
- Evidence outside its short review window, duplicate/missing check IDs,
  release/policy fingerprint drift or nonzero cleanup counts fail or hold.
- No record field accepts prompt, Tool arguments/results, stdout/stderr,
  Workspace/Skill/Artifact body, URL, host path, credential, token or Secret
  value.
- The validator reads only the supplied policy and record; it does not mutate
  PostgreSQL, MinIO, host services or protected runtime state.

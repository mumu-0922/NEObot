# G20.8 shadow and canary boundaries

## Comparable patterns

- Shadow traffic duplicates an authorized read request but discards the shadow
  response from the user-facing path.
- Canary rollout starts with deterministic allowlisted cohorts, explicit error/
  latency budgets and a fast kill switch; evaluation never grants admission.
- Dry-run controllers execute validation and read-only observation but cannot
  commit external effects.

## Repository constraints

- Production Agent Runtime, Scheduler and Learning workers are disabled.
- Exact-host verification currently fails closed with
  `ISOLATION_UNAVAILABLE`; G20.8 source work must not turn that into an
  in-process package executor.
- G20.4 Broker already models side-effect approval/Prepare/Commit and G20.5
  caps Child depth at one. Shadow must not bypass either boundary.
- PostgreSQL is authority; Redis may wake a worker later but cannot own cohorts,
  observations or budgets.

## Recommended contract

- Add a default-off server-owned Shadow policy with administrator enablement,
  explicit user opt-in, deterministic cohort membership, exact admitted package
  and runtime fingerprints, start/end time, revision and bounded budgets.
- `synthetic` and `read_only` are the only G20.8 modes. Both require a Grant
  whose effective capabilities exclude write, Secret, Egress and delegation.
- Shadow output is never injected into Chat, never mutates Run/Cron/install/
  Draft authority and is never promotion authority.
- Persist only content-free observations: policy/cohort/run/package
  fingerprints, counts, latency buckets and stable reason codes. Do not store
  prompt, Tool bodies, package output, Artifact content or Secret values.
- Missing exact-host acceptance keeps executable canary unavailable. Synthetic
  contract replay may run through injected test adapters; package code is never
  executed in the API process.
- Budget breach, Kill Switch, user opt-out, package/runtime drift or restart
  fences new work immediately. Old generations cannot publish observations.

## Promotion boundary

Passing source tests does not promote G20.8. Promotion additionally requires
the exact deployment isolation suite, clean-copy/restart, paired backup/restore
and observed cohort budgets. Until that evidence exists, the UI reports the
held reason and legacy execution remains authoritative.

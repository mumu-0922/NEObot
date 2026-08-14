# G21 Agent Runtime Production Activation Plan

## Baseline

G20.1-G20.10 and migrations `083`-`090` remain the authority. Legacy text
Skills are retired and admitted Package Skills are the only eligible Skill
domain. G21 advances production capability one independently evidenced stage
at a time; no later stage may be inferred from an earlier gate.

The development host remains `ISOLATION_UNAVAILABLE`. Exact-host artifacts,
credentials and live records stay outside Git. Every stage is default-off,
rollbackable by disabling its own worker/flag, and forbidden from widening a
frozen Grant or reviving browser/API/rootful execution.

## G21.0 — Exact-host bundle and control-plane maintenance

Status: source/control implementation complete; target-host activation held.

- Build and verify a deterministic content-addressed `neo-runnerd`/
  `neo-runner-probe` deployment bundle.
- Harden literal private/loopback bind and systemd probe/state boundaries.
- Require fresh `control_plane` activation evidence for exact release, policy,
  mTLS identities and five live checks.
- Run a separate least-privilege worker for `probe`, `list` and `reconcile`
  only. Keep Root Runs, Broker, Child, Cron and Learning false.
- Add default-off Compose/preflight wiring with a sixth PostgreSQL principal.

Promotion gate: run `scripts/verify-agent-runtime-g21-0.sh`, then reproduce a
real `ACTIVATION_READY` record on the approved target. Offline fixtures and the
current host cannot promote.

## G21.1 — Root Run launch canary

Status: source/control implementation complete; exact-host canary held.

- Add the independent `root_run_canary` activation stage, immutable canary
  plan, mTLS identity and Ed25519 authority key binding.
- Give Runner control and canary identities disjoint ingress method policies;
  control cannot launch and canary cannot call Broker Prepare/Commit.
- Run one idempotent synthetic depth-zero Run with an empty Tool Registry,
  read-only rootfs, `networkMode=none`, no Egress/Secret and bounded resources.
- Prove claim, signed launch, Runner/PostgreSQL heartbeats, signed cancel, exact
  reap and zero post-cancel Runner inventory.
- Atomically terminalize Sandbox, Attempt, Step and Run with three append-only
  cancellation events; roll back every projection on identity/state drift.
- Fence tokenless restart recovery until lease expiry and prefer exact signed
  cancel plus durable cancellation after a known-running heartbeat failure.
- Add a seventh LOGIN inheriting exactly `agent_orchestrator_runtime` and
  `agent_runner_control`, without adding migration `091` or direct table DML.
- Keep the dedicated Compose profile default-off and credential-free beyond
  its exact database, mTLS, activation, plan and signing-key inputs.

Promotion gate: run `scripts/verify-agent-runtime-g21-1.sh`, then reproduce a
fresh `ROOT_RUN_CANARY_GATES_PASSED` activation record and full canary flow on
the approved exact host. The disposable PostgreSQL proof and current-host
`ISOLATION_UNAVAILABLE` result cannot promote user-facing execution.

## G21.2 — Read-only Broker and Artifact canary

Wire only reviewed read-only Project/Workspace/MCP actions plus bounded Artifact
publication. Prove Prepare/Commit replay, stale lease denial, object-before-row
cleanup, no credential in Sandbox and `outcome_unknown` no-retry handling.
Mutable effects remain false.

## G21.3 — Bounded mutable effects

Activate individual mutable actions by exact Grant/policy after approval,
idempotency and crash matrices pass. Rollout is action-by-action; no generic
network, vault or MCP write capability is admitted.

## G21.4 — Depth-1 Child Agents

Enable Child launch only after physical `delegate_task` removal, strict
Grant/Registry/budget subsets, atomic reservation, Child-first cancel/reap and
restart reconciliation pass on the exact target. Depth two remains impossible.

## G21.5 — Cron and Draft learning workers

Activate durable Cron scheduling and Draft-only learning independently. Prove
DST/missed/overlap/retry/revocation for Cron and provenance/check/human Promote/
cleanup for Drafts. Learning never mutates an installed package in place.

## G21.6 — Product canary and final closure

Expose a bounded eligible cohort only after all prior stages hold current
evidence. Run clean-copy/restart/reboot, paired backup/restore, disaster
recovery, rollback/forward-fix, rotations, alerts, capacity and final cleanup.
The exact release must evaluate to `PROMOTION_READY`; temporary evidence is
removed while content-free incident/audit authority remains.

## Epic completion

- Every enabled package Run uses the exact accepted rootless Runner path.
- Every action is Grant/lease/idempotency/Kill-Switch fenced.
- Child depth is at most one; Cron and Learning cannot create authority.
- No legacy, browser, API in-process, rootful Docker or public Runner fallback
  exists.
- Backup/restore, clean restart, rollback and current evidence reproduce from a
  clean baseline.

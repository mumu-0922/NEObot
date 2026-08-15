# G21.5 independent activation and rollback research

## Question

How should Cron and Draft-learning be deployed, gated and rolled back without
turning the development host into a production Runner or coupling the stages?

## Repository findings

- G21.0-G21.4 use separate commands, Compose profiles, LOGINs, mTLS callers,
  plan files, activation records and enabled-preflight gates. All profiles are
  absent from default Compose startup.
- The checked-in environment template keeps broad Runtime, Scheduler, Skill
  install, Learning, delegation and Broker mutation switches false.
- The current host correctly reports `ISOLATION_UNAVAILABLE`; prior stages do
  not install Podman, create accounts/sub-IDs/cgroups/systemd, provision live
  secrets or manufacture production evidence.
- Cron does not need Runner credentials: it materializes and enqueues a normal
  Run. Draft isolation/evaluation does need a distinct Runner lifecycle caller,
  but no database/object-store credential crosses into `neo-runnerd` or a
  Sandbox.

## Recommended rollout

Use two independent stages:

- `cron_worker`: `agent-runtime-cron-worker` command, Compose profile,
  `AGENT_CRON_WORKER_ENABLED`, exact plan/activation target and a LOGIN
  inheriting only `agent_cron_worker`.
- `draft_learning_worker`: `agent-runtime-draft-learning-worker` command,
  Compose profile, `AGENT_DRAFT_LEARNING_WORKER_ENABLED`, exact plan/activation
  target, object-store read/delete authority, a LOGIN inheriting only
  `agent_learning_worker`, and a distinct Runner mTLS caller/key set.

Neither flag implies the other. Broad `AGENT_RUNTIME_ENABLED`,
`AGENT_SCHEDULER_ENABLED`, `AGENT_LEARNING_ENABLED`, Broker mutation,
delegation and product cohort switches remain false. Each stage requires fresh
G21.0-G21.4 readiness plus its own production-class evidence at migration head
094.

The Cron stage may run without the learning stage or Runner. The learning stage
may run without Cron but requires the approved exact-host Runner and pre-staged
Draft-check Workspace. The development host only verifies templates and must
continue returning `ISOLATION_UNAVAILABLE`.

## Preflight

- Validate strict plan and activation schemas, secure regular files, exact Git
  commit/migration head/policy/manifest/endpoint/certificate/key bindings and
  non-placeholder material.
- Verify independent flags, profile selection, distinct LOGIN inheritance and
  absent foreign service credentials.
- Verify the operator-provisioned database activation target matches the plan
  before opening claims.
- Require zero stale target claims/Sandboxes before starting each worker.

## Rollback

- Stop and disable only the affected profile and database activation target.
- Cron rollback stops new claims, releases/reclaims its exact cursors/triggers
  and leaves already enqueued normal Runs under Orchestrator policy.
- Learning rollback stops new check claims, cancels/reconciles its exact Runner
  attempts, but keeps cleanup/reconciliation callable through a reviewed
  maintenance invocation.
- Keep migration 094 applied in production. Down is disposable-only and refuses
  active targets, claims, Runner checks, unresolved cleanup or retained
  activation evidence.
- Restart or restore requires fresh activation evidence and treats pre-restore
  leases/Runner inventory as untrusted.

## Alternatives rejected

- One combined worker/profile: couples failure and rollback domains and forces
  Cron to receive Runner/object-store credentials.
- Enable the global Scheduler/Learning switches: silently broadens to all
  existing Templates/Drafts.
- Promote this development host: contradicts current live isolation evidence.

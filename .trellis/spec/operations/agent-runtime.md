# Agent Runtime Operations Contract

## Scenario: Deploy and operate `neo-runnerd` with rootless OCI

### 1. Scope / Trigger

Apply this contract when changing Agent Runtime host prerequisites, Runner
service/account/mTLS, OCI images, Workspace/Scratch/Artifact boundaries,
capability probe, isolation tests, Kill Switches, release, backup/restore or
legacy Skill cutover. Protect `.env.single-server`, `data/`, `secrets/` and
`backup/`.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-runtime-phase0.sh
bash mm-chat/scripts/verify-agent-orchestrator.sh
bash mm-chat/scripts/verify-agent-orchestrator-postgres17.sh
bash mm-chat/scripts/verify-agent-runner.sh
bash mm-chat/scripts/verify-agent-runner-postgres17.sh
bash mm-chat/scripts/verify-agent-broker.sh
bash mm-chat/scripts/verify-agent-broker-postgres17.sh
bash mm-chat/scripts/verify-agent-delegation.sh
bash mm-chat/scripts/verify-agent-delegation-postgres17.sh
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero until exact host is prepared
```

The Runner, Broker and delegation gates prove source/control and disposable
PostgreSQL behavior. The host gate must return `ISOLATION_UNAVAILABLE` here and
becomes promotion evidence only when the exact approved service account/release
passes the full suite.

### 3. Contracts

- `neo-runnerd` is a dedicated host-side non-root service, separate from
  `mcp-runner`, and has no browser bearer, database, Redis, object-store,
  Provider vault or Docker/Podman socket credential.
- Every Run gets a fresh rootless OCI Sandbox: nonzero UID/GID, read-only root,
  empty capabilities, `no_new_privs`, reviewed seccomp, cgroup v2 CPU/memory/
  PID/time/output limits, no host PID/IPC/network/project bind/device/socket.
- The exact Runner service account must pass userns/subuid/subgid, cgroup v2,
  seccomp, storage/network and kill/reap capability probes. Docker daemon health
  is not proof; rootful/privileged/sudo fallback is forbidden.
- Runner RPC is private mTLS with version, deadline, body/concurrency bounds,
  nonce/request replay fence and exact lease/snapshot fingerprints.
- Project Workspace is a fully rehashed read-only snapshot. Scratch is a
  size-bounded tmpfs and host broker staging is always destroyed. Artifact uses
  a per-Attempt framed Unix intake into Runner quarantine. Sandbox never
  receives object-store credentials or arbitrary host mounts.
- Egress is `none`, exact allowlist or Tool-specific broker. Re-resolve/recheck
  DNS, redirects and reconnects; deny localhost/private/link-local/metadata/raw
  IP forms and origin-changing credential forwarding.
- Secret Broker injects no durable/env/prompt value; use action-scoped short-TTL
  handles or Broker-side credential application and prove zero leakage.
- Runtime disabled still runs retention, cleanup, expired-intent removal and
  orphan reconciliation.
- Migration `084` supplies database-only Kill Switch fencing, recovery,
  projection rebuild and retention before any Runner exists. A passing G20.2
  drill is not rootless isolation evidence and must not enable Runtime.
- Migration `085` supplies Runner/lease-owner-bound replay and expected Sandbox
  lifecycle authority through `agent_runner_control`; the host daemon has no DB
  role. Cleanup/reconcile and completed replay pruning remain usable while
  execution is disabled.
- Migration `086` supplies durable Prepare/approval/Commit/receipt and
  secret-handle-digest authority through `agent_effect_control`. The Runner,
  public API and Orchestrator roles gain no table DML. Its default production
  relay remains unavailable; do not manually invoke database functions as an
  executor or promote the deterministic Project CAS fake.
- Migration `087` supplies held root/child lineage, Parent reservations,
  launch admission, settlements and durable reap work through
  `agent_delegation_control`. API, Orchestrator, Runner and effect roles gain no
  delegation DML; `neo-runnerd` remains credential-free. No public/startup Child
  worker exists, and operators must not manufacture root authority or Child Runs
  in a live database.
- Parent cancel/kill and reconcile fence Child leases plus terminal
  Attempt/Step/Run state before host reaping. Reap failure remains durable and
  retryable; never restore a lease or delete delegation facts to clear health.
- A possibly sent mutable effect must be reconciled by its exact stable status
  key. Without exact committed/not-sent proof, record `outcome_unknown`; never
  retry with another key. Runtime-off operation still expires intents/revokes
  handles and permits receipt reconciliation and cleanup.
- Pre-Commit cancellation uses the migration `086` append-only cancellation
  authority and races under the same exact intent lock as Commit. A cancellation
  winner terminalizes the prepared Attempt and performs zero executor calls; a
  `committing`/terminal intent rejects cancellation and follows receipt or
  `outcome_unknown` recovery instead.
- Grant revocation remains append-only. The control service must pair its
  durable revocation with immediate zeroization of matching memory-only Secret
  bytes; the database revokes every active durable handle digest.
- Final legacy cutover is hard deletion only after verified backup, clean-copy,
  restart, history-label and rollback rehearsal. Never mix dual execution.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| rootless runtime/feature/version missing | Runner not ready; no rootful fallback |
| mTLS/version/replay failure | reject without launch or secret output |
| OCI config requests root/capability/host mount/network | reject before create |
| stale/unknown Sandbox after restart | kill exact cgroup and reconcile |
| cgroup/output/time budget exceeded | terminate exact Sandbox; Runner stays healthy |
| Kill Switch `kill` | fence lease/effects, kill descendants, remove Scratch |
| restore sees pre-restore live Sandbox | kill; never trust old lease/nonces |
| cleanup object removal fails | retain durable queue/state and retry |
| Child reap fails after Parent cascade | keep Child terminal/lease-fenced; retry exact durable reap |
| migration `087` passes but exact host is held | keep production Child execution disabled |

### 5. Good / Base / Bad Cases

- **Good**: pinned Runner/runtime on a prepared host passes the exact Isolation
  Acceptance Suite, then a no-network/no-secret read-only canary runs.
- **Base**: Phase 0 verifier passes while no Runner/runtime is installed or
  enabled; output explicitly says isolation acceptance was not executed.
- **Bad**: `docker run --privileged`, mounting `/var/run/docker.sock` or source,
  adding `CAP_SYS_ADMIN`, host networking, secrets in env, or calling a healthy
  container proof of isolation.

### 6. Tests Required

- Capability probe and OCI config inspection as exact Runner account.
- Filesystem/mount/socket/device/procfs/symlink/hardlink/path-race negatives.
- DNS rebinding/redirect/reconnect/metadata/private-IP/credential-leak negatives.
- Secret canary across env/argv/proc/prompt/output/files/artifacts/logs/metrics.
- CPU/memory/PID/disk/output/wall exhaustion and descendant/orphan/reboot reap.
- Lease reclaim, Prepare/Commit crash matrix, depth-1 Registry and Runtime-off
  cleanup proof.
- Migration `087` concurrent reservation, stale Parent/Child launch, terminal
  settlement, cascade/reap failure, terminal/expired/reclaimed/Kill-Switch
  recovery, least privilege, dump/restore and clean down/up; all older tail
  drills must return to `087` head.
- Paired PostgreSQL/object backup, restore-with-Runtime-off and reconciliation.
- G20.1 backup/restore pairs migration `083` rows with all three immutable
  object prefixes: `skill-quarantine/`, `skill-packages/`, and `skill-sboms/`.
  The temporary MinIO drill exports PostgreSQL coordinates and `mc stat`s every
  sampled key before cleanup.

### 7. Wrong vs Correct

#### Wrong

```text
Docker works -> start a privileged container -> call Sandbox complete
```

#### Correct

```text
exact non-root account probe -> immutable launch envelope -> rootless OCI
-> broker-only I/O -> fingerprint-bound acceptance evidence -> bounded canary
```

## Scenario: Operate the held Cron control plane

### 1. Scope / Trigger

Apply this contract when deploying migration `088`, assigning Cron database
roles, running Cron verification, reconciling expired scheduling claims,
pruning Cron history or rolling the migration back. This scenario does not
authorize a production Scheduler service.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-cron.sh
bash mm-chat/scripts/verify-agent-cron-postgres17.sh
```

Migration `088_agent_cron_foundation` and `agent_cron_control` are the durable
operational boundary. The exact-host gate remains expected-nonzero
`ISOLATION_UNAVAILABLE` until the separate Runner promotion criteria pass.

### 3. Contracts

- Install migration `088` with a NOLOGIN owner and the dedicated
  `agent_cron_control` role. That control role has SELECT plus exact function
  execution and no table DML or membership in the owner role; API,
  Orchestrator, Runner, Broker and delegation roles gain no Cron authority.
- Keep Runtime and Scheduler disabled. Do not add an HTTP/Chat/frontend/startup
  route, Redis wake loop, Compose worker or production execution wiring merely
  because the source and PostgreSQL gates pass.
- Reconciliation may clear expired cursor claims and return expired trigger
  claims to pending. Cleanup must use bounded `agent_cron_prune` batches and
  retain live/enqueued Run facts until eligible.
- Pause/resume/delete through controlled lifecycle functions. Resume advances to
  the next future instant and records only sanitized missed-window facts; never
  manufacture a backfill directly in tables.
- Keep `.env.single-server`, `data/`, `secrets/` and `backup/` untouched. Cron
  rows and audits must not contain prompts, Secret values, Workspace bytes,
  Tool arguments/results or credentials.
- The down migration is deliberately guarded by
  `AGENT_CRON_DOWN_DATA_EXISTS`. Drain/tombstone and boundedly prune eligible
  state before an intentional rollback; never delete protected runtime state or
  bypass the guard.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| production Scheduler/Runtime switch remains off | no schedule worker starts; reconcile/prune remain available |
| stale/expired claim worker attempts mutation | `STALE_CLAIM`; operator reconciles rather than forcing table DML |
| trigger loses current authority | sanitized Cron denial fact; no Run enqueue |
| `agent_cron_control` obtains table DML or another runtime role obtains Cron access | deployment gate fails; do not promote |
| down attempted while Cron data/audit remains | `AGENT_CRON_DOWN_DATA_EXISTS`; rollback stops intact |
| source/PG17 gates pass but exact host is unprepared | keep production disabled; host result remains `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good**: deploy `088`, prove least privilege and restart/idempotency on a
  disposable PostgreSQL 17 database, then leave production scheduling disabled.
- **Base**: operators run bounded reconcile/prune while Runtime is off; retained
  Cron and Orchestrator history remains auditable.
- **Bad**: grant direct Cron table UPDATE, manually relink an occurrence, delete
  audit rows to force down, or treat a disposable-database pass as Scheduler or
  exact-host promotion evidence.

### 6. Tests Required

- Run both Cron signatures plus Phase 0, focused race/vet tests and
  `go mod verify`.
- PostgreSQL 17 must prove fresh/replay, concurrency, restart reclaim,
  acknowledgement-loss idempotency, bounded retry/overlap, all authority
  denials, least privilege, content-free dump/restore, guarded down and clean
  down/up.
- Every prior PostgreSQL tail drill must peel `088` before testing its older
  guard and finish reapplied at head `088`.
- The full standalone gate must pass, while the exact-host Runner gate remains
  expected-nonzero `ISOLATION_UNAVAILABLE` unless a separately approved host
  promotion is in scope.

### 7. Wrong vs Correct

#### Wrong

```text
PG17 Cron drill passed -> add startup Scheduler -> enable production triggers
```

#### Correct

```text
deploy guarded migration 088 -> prove narrow role + durable replay/recovery
-> keep Scheduler/Runtime disabled -> promote only in a later explicit gate
```

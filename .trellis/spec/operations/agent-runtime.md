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
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero until exact host is prepared
```

The first two Runner gates prove source/control and PostgreSQL behavior. The
host gate must return `ISOLATION_UNAVAILABLE` here and becomes promotion
evidence only when the exact approved service account/release passes the full
suite.

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

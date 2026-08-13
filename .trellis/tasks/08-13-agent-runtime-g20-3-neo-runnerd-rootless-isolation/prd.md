# Agent Runtime G20.3 — `neo-runnerd` and Rootless Isolation

## Goal

Implement the dedicated non-root Agent Runner boundary: versioned mTLS Runner
RPC, PostgreSQL-durable replay/Runner lease authority, fail-closed exact-release
capability probing, one fresh rootless OCI Sandbox per Attempt, exact
kill/reconcile/cleanup, immutable Workspace input and credential-free Artifact
intake. The development host may remain unready; neither a fake driver nor
rootful Docker may be reported as production isolation evidence.

## Shared Baseline

- All prior G20 design decisions remain binding. PostgreSQL 17 is the only
  durable Run/Step/Attempt/lease/Kill-Switch/replay authority.
- The approved target stack is exact Podman 6.1.0 + crun 1.29.1 with the
  release-manifest versions recorded in research. Runtime lookup never accepts
  floating `latest` or an arbitrary `PATH` substitute.
- `neo-runnerd` runs as a dedicated non-root identity outside application
  containers. It never receives Docker/Podman socket, PostgreSQL, object-store,
  model Provider or vault credentials.
- Production launch is impossible unless the exact capability probe is ready.
  The current WSL2/Jammy development host is expected to fail readiness because
  the exact stack, uidmap, seccomp and delegated cgroup environment are absent.
- Network mode is `none` in G20.3. Brokered Egress/Secret/Tool and mutable
  Prepare/Commit are deferred to G20.4.
- Runtime remains globally unavailable to users: no Agent HTTP/Chat/frontend
  route, API startup worker or legacy Skill behavior is enabled.
- Current pure-text Skills remain unchanged through G20.8 and are hard deleted
  without migration/wrapping only in G20.9.
- User selected all recommended choices and authorized verified commits without
  another confirmation prompt. No sub-Agent delegation is used.

## Requirements

### Release manifest and capability probe

- Add a checked-in machine-readable Runner release manifest binding exact
  protocol, binary versions, executable fingerprints/approved paths, storage,
  network, seccomp and probe-suite fingerprints.
- Probe as the exact Runner process identity. Refuse UID 0, incomplete or
  overlapping subordinate IDs, missing `newuidmap`/`newgidmap`, non-cgroup-v2,
  missing delegated CPU/memory/PID controllers, unavailable seccomp, missing
  pidfd/exact reap primitive, mutable/unapproved release files, or version/hash
  drift.
- Probe produces bounded content-free evidence and one canonical fingerprint.
  It never exposes host paths, private sub-ID coordinates, environment, keys or
  source/package/Workspace content.
- Docker availability, fake drivers and command-intent inspection never set
  production readiness true.

### Versioned mTLS Runner RPC

- Add `cmd/neo-runnerd` and an isolated internal Runner package. The daemon
  refuses root and starts only on an explicitly private/loopback address with
  TLS 1.3 mutual authentication, pinned trust root and exact allowed client
  identity.
- Strictly implement G20.3 methods: `probe`, `launch`, `heartbeat`, `cancel`
  and `list/reconcile` using `neo.runner-rpc/v1`; bound request size, headers,
  deadline, clock skew, concurrency and response fields.
- Unknown fields, duplicate keys, method/body mismatch, invalid timestamps,
  unsupported versions, unauthorized certificate identity and plaintext fail
  closed with stable sanitized errors.
- `prepare`/`commit` remain unavailable in G20.3 even though their future schema
  shapes already exist.

### Durable replay and Runner authority

- Add migration `085` with control-plane authority-ticket/replay and expected
  Sandbox lifecycle projection sufficient for restart/reconciliation. The
  daemon receives no database credential.
- Bind PostgreSQL and Runner-local replay keys to caller certificate identity +
  request ID + nonce + exact request fingerprint. The Control Plane persists
  and signs a short-lived exact authority ticket; the daemon verifies it and
  fsync-claims a mode-0600 host-local ledger before driver action. Restart of
  either side cannot reset the fence; mismatch or duplicate inflight requests
  return `REPLAY_DETECTED`; exact completed replay returns the prior sanitized
  response without repeating the OCI action.
- Launch/heartbeat/cancel bind exact G20.2 Run, Step, Attempt, generation,
  lease owner/token digest, snapshot fingerprint and current Kill Switch epoch.
  Expired/reclaimed/stale credentials never reach the driver.
- Use least-privilege `agent_runner_owner`/`agent_runner_control` roles with
  exact `SECURITY DEFINER` functions. The future Backend control-plane worker
  may assume the narrow control role; `neo-runnerd` never does. No role has
  direct table DML; `go_api_runtime` and `agent_orchestrator_runtime` gain no
  broad Runner table rights.
- Cleanup/reconciliation and expired replay pruning remain callable while
  execution is disabled or a Kill Switch is active. Down migration refuses
  while Runner authority exists.

### Rootless OCI lifecycle

- Implement a Podman command driver behind a narrow testable interface. Create
  one deterministic, label-bound container per Attempt, then inspect before
  start. Never use shell interpolation, `podman run`, Docker, `sudo`, setuid
  fallback, privileged mode, host network/PID/IPC, runtime socket, arbitrary
  device or arbitrary bind.
- Enforce nonzero container UID/GID, per-Sandbox user namespace, read-only
  rootfs, immutable digest image, empty capabilities, `no-new-privileges`, exact
  seccomp fingerprint/profile, `network=none`, cgroup v2 CPU/memory/PID limits,
  wall/output limits and bounded immutable argv.
- The only workload mounts are Runner-owned verified Workspace (read-only),
  Runner-owned per-Attempt Scratch (read/write), and narrow broker channels.
- Treat CLI flags as requested intent. Launch succeeds only after strict
  machine-readable inspection proves the generated container config matches
  every frozen invariant and expected label/fingerprint.
- Kill/reap targets the exact recorded container ID/cgroup/process identity,
  waits for descendants to disappear, removes the container and cleans Scratch,
  broker partials and staging. Unknown/stale/mismatched containers are killed
  fail closed during restart reconciliation.

### Workspace, Scratch and Artifact boundary

- Add a Runner-owned content-addressed Workspace catalog/materializer. RPC may
  reference only a snapshot ID/fingerprint, never an arbitrary host path.
- Safely materialize a bounded tar stream: reject absolute/traversal/NUL,
  duplicates and Unicode/case collisions, symlink/hardlink/special files,
  oversized content/tree and fingerprint mismatch. Atomic promotion yields a
  read-only verified snapshot.
- Scratch is exact-Attempt, mode 0700, quota-bound and removed on terminal,
  kill, failed launch, stale/orphan reconcile and daemon restart cleanup.
- Add a narrow local Artifact intake seam that receives bounded bytes without
  object-store credentials, validates logical metadata, streams to Runner-owned
  quarantine and fingerprints content. It cannot publish/attach artifacts and
  rejects stale Attempt authority. Partial/quarantined data is cleaned on all
  lifecycle failures.

### Diagnostics, operations and verification

- Logs/evidence contain stable IDs, fingerprints, versions, feature/error
  classes and bounded counts/durations only. They exclude lease tokens, raw
  request bodies, argv, stdout/stderr, content, host paths, sub-ID details,
  certificates/keys and credentials.
- Add a source/offline verifier, disposable PostgreSQL 17 replay/least-
  privilege/dump-restore drill, fake-driver lifecycle/replay/cleanup acceptance
  suite, and exact-host isolation probe command.
- The exact-host command exits nonzero with `ISOLATION_UNAVAILABLE` on the
  current host. This is a correct fail-closed result, not a waived gate. A
  production-ready marker is emitted only by the exact service account/stack
  passing the complete release-bound suite.
- Add systemd/release templates and an operator installer/preflight contract,
  but do not modify the host, create accounts, install packages, write live
  secrets, or add `neo-runnerd` to application Docker Compose.

## Internal Service Surface

```text
Probe / Ready
IssueAuthority / CompleteRequest / PruneReplay
ClaimLocalRequest / CompleteLocalRequest / CompactLocalReplay
Launch / Heartbeat / Cancel / ListSandboxes / Reconcile
RegisterWorkspace / ResolveWorkspace / RemoveWorkspace
BeginArtifact / WriteArtifact / FinalizeArtifact / AbortArtifact
```

All production mutation surfaces are internal mTLS control-plane methods; none
is a user-facing Runtime API.

## Acceptance Criteria

- [ ] Root execution, plaintext/public listener, TLS client mismatch,
      unsupported version, malformed/oversized/duplicate-field request and
      timestamp/nonce replay all fail before driver action.
- [ ] Exact completed replay returns the prior result; inflight/mismatched
      replay is rejected across daemon-local-ledger and PostgreSQL restart.
- [ ] Launch with stale lease/generation/token, snapshot drift, active Kill
      Switch or unready/drifted probe performs zero driver actions.
- [ ] Generated Podman create intent and inspected config prove exact digest,
      userns, UID/GID, mounts, read-only rootfs, no caps/no-new-privileges,
      seccomp, network-none and CPU/memory/PID/wall/output limits.
- [ ] Cancel/kill, daemon crash recovery, unknown/orphan Sandbox and lease
      reclaim target only the exact Sandbox and remove Scratch/broker/staging
      residue.
- [ ] Workspace malicious archive corpus and path-race cases cannot escape or
      introduce link/special-file authority; fingerprint mismatch is atomic.
- [ ] Artifact intake has no object-store credential, is size/type/name/fence
      bound and never publishes bytes; stale/partial paths are cleaned.
- [ ] Migration `085` passes PostgreSQL 17 fresh/replay/non-empty-down refusal,
      least privilege, replay concurrency, retention and dump/restore.
- [ ] Focused race/vet/tests, all Backend tests, frontend/RAG gates, Phase 0 and
      full standalone clean-copy gate pass.
- [ ] Exact-host acceptance fails closed on this development host and records
      only sanitized missing-feature/drift classes; no fake/rootful evidence is
      promoted.
- [ ] Agent Runtime/Chat/API/frontend and legacy text Skills remain unavailable
      and unchanged.

## Deliverables

- `mm-chat/backend/cmd/neo-runnerd/` daemon and tests.
- `mm-chat/backend/internal/agentrunner/` RPC, replay, probe, lifecycle,
  workspace/artifact, Podman driver, tests, `README.md` and `DESIGN.md`.
- `mm-chat/backend/migrations/085_agent_runner_foundation.{up,down}.sql` plus
  schema and PostgreSQL integration coverage.
- `mm-chat/config/agent-runner/` exact release/seccomp configuration and
  `mm-chat/deploy/agent-runner/` systemd/release templates.
- `mm-chat/scripts/verify-agent-runner*.sh` and release-bound isolation report.
- Agent Runtime architecture/contract/deployment/tracking/Trellis specs updated
  to the implemented boundary and explicit promotion hold.

## Out of Scope

- Installing/changing host packages, service accounts, subuid/subgid, cgroups,
  systemd, live certificates/secrets or live runtime storage.
- G20.4 Tool/Egress/Secret Brokers, approvals and Prepare/Commit side effects.
- Artifact publication/object storage, mutable Project patches and Chat output.
- Public Agent Run routes, frontend UX, Scheduler/Cron, Child Agents, learning,
  Runtime enablement, Chat integration and legacy Skill deletion.

## Decision (ADR-lite)

**Context:** A source implementation can be verified on this host, but the
approved rootless release prerequisites are absent and rootful Docker would be
false evidence.

**Decision:** Implement the complete fail-closed Runner control/lifecycle
boundary against an exact Podman/crun release manifest; use fakes only for
deterministic negative/lifecycle tests and retain an explicit production
promotion hold until the release-bound host suite passes as `neo-runner`.

**Consequences:** G20.4 can build on a stable, credential-free isolation seam;
no production execution is unlocked by this task; operator host provisioning
and exact-host proof remain a separate release action rather than an implicit
developer-machine mutation.

## Research References

- [`research/rootless-stack-and-host.md`](research/rootless-stack-and-host.md)
- [`research/runner-rpc-and-replay.md`](research/runner-rpc-and-replay.md)
- [`research/workspace-artifact-and-lifecycle.md`](research/workspace-artifact-and-lifecycle.md)

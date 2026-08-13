# Agent Runner Design

## Authority chain

```text
G20.2 Attempt lease + snapshot + Kill Switch epoch
  -> PostgreSQL `agent_runner_issue_authority`
  -> short-lived Ed25519 ticket bound to Runner ID == lease owner
  -> TLS 1.3 client identity and strict Runner RPC envelope
  -> mode-0600 fsync local replay claim
  -> exact release probe
  -> Workspace re-hash + OCI create/inspect/start
```

PostgreSQL is the durable control authority. `neo-runnerd` has no database,
object-store, Provider or vault credential. Exact completed RPC replays return
the previous sanitized response; inflight or mismatched replays perform no OCI
action.

The ticket signs request ID, nonce and an authority fingerprint computed from
the method body with the ticket field cleared. This avoids a circular signature
while preventing the signed proof from being moved to a different payload.

## Isolation proof

Podman flags are only requested intent. Launch inspects the created container
and compares exact image digest, container identity, labels, OCI runtime,
private user namespace maps, nonzero UID/GID, read-only rootfs, empty effective
and bounding capabilities, exact no-new-privileges/seccomp options, network/PID/
IPC/UTS modes, cgroup manager/mode/parent, CPU/memory/PID limits, log driver,
bounded `/scratch` tmpfs and exact Workspace/Broker mounts. Inspection repeats
after start.

Cancellation uses the exact container ID, stop/kill, wait and remove sequence,
then verifies the recorded PID and cgroup identity disappeared before deleting
state. Reconciliation scans driver inventory plus local records and removes
unknown Scratch/Artifact staging. Wall deadlines reap a Sandbox locally even if
the control plane stops heartbeating.

## File boundaries

- Workspace tar input rejects traversal, links, special files, duplicates and
  case/Unicode collisions. Every Resolve rewalks, owner/mode-checks and rehashes
  the full bounded tree.
- Scratch is a size-bounded tmpfs inside the Sandbox. The host Attempt directory
  holds only the narrow broker socket/staging and is removed on every failure or
  terminal path.
- Artifact intake accepts one length-framed, bounded metadata+byte stream on a
  per-Attempt Unix socket. It rechecks current Attempt generation, hashes to a
  Runner-owned quarantine and cannot publish or attach the result.

## Held boundary

G20.3 implements source/control foundations only. The checked-in release
manifest remains unapproved; current exact-host readiness is false. G20.4 owns
Tool/Egress/Secret Brokers and Prepare/Commit. Public Runtime/API/Chat and legacy
text-Skill behavior remain unchanged.

# `agent-runtime-local-test-smoke` design

## Goal

Prove that the current WSL test host can execute and reap one harmless Skill
through the same immutable isolation inspection used by `neo-runnerd`.

## Non-goals

- Production readiness, activation, closure, or promotion.
- API, Chat, Agent Center, database, Broker effect, Child, Cron, or learning
  wiring.
- Real prompts, user files, network access, credentials, or secrets.

## Flow

```text
pinned local Podman info
  -> disposable local OCI image
  -> synthetic read-only Workspace
  -> ephemeral Ed25519 launch authority
  -> real Service launch + create/inspect/start/inspect
  -> workload checks + framed Artifact result
  -> signed result + kill/reap
  -> image, Sandbox, Scratch, Artifact, Workspace cleanup
  -> local_test-only content-free report
```

The command deliberately uses `StaticProbe` only after the local host manager
has verified the pinned toolchain and live Podman tuple. That probe cannot be
serialized as an approved production release or Isolation Acceptance report.
Every mutable item is synthetic and lives below the dedicated local-test root.

## Security decisions

- Every input has its fixed name below one private local-test root. Input files
  are absolute regular non-group/world-writable files; root/config/state
  directories must stay private, and every symlinked path is rejected.
- The workload runs as UID/GID `10001`, with an empty Tool Registry, read-only
  rootfs, empty capabilities, `no-new-privileges`, reviewed seccomp,
  `network=none`, and exact CPU/memory/PID/wall/output/scratch limits.
- The workload publishes only fixed content-free check names through the real
  per-Attempt Unix Artifact intake.
- Post-create and post-start inspection remains authoritative; CLI flags are
  not treated as proof.
- Failure attempts best-effort signed reap and remove only the dedicated
  disposable image/state. Success requires zero managed Sandbox and staging
  residue.
- The report schema fixes `evidenceClass=local_test` and
  `productionEligible=false`; production evaluators reject it.

## Known limit

This proves the host and one bounded lifecycle. It does not connect the page to
the held production worker chain. The current desktop user owns the rootless
engine, which is acceptable only for this local test profile.

## Change history

- 2026-08-15: initial WSL2 local-test smoke.

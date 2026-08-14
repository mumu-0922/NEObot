# Agent Runtime G21.0 — Exact-host deployment and control wiring baseline

## Goal

Deliver a content-addressed exact-host `neo-runnerd` deployment bundle and the
first production wiring stage: a dedicated, least-privilege control worker that
can validate fresh target-host activation evidence and perform only mTLS
probe/list/reconcile maintenance while all Agent execution remains disabled on
the current development host.

## Shared baseline

- G20.1-G20.10, migrations `083`-`090`, Package-Skill-only authority and legacy
  text-Skill deletion remain binding.
- The user approved all recommended decisions, direct verified commits and no
  additional preference questions. No sub-Agent delegation is used.
- The current development host remains `ISOLATION_UNAVAILABLE`; no host package,
  service account, sub-ID, cgroup, systemd or credential mutation is allowed.
- Never touch `.env.single-server`, `data/`, `secrets/` or `backup/`.
- No browser/API/rootful Docker fallback, public Runner endpoint or credential
  injection into Sandboxes is permitted.

## Requirements

### Exact-host deployment bundle

- Add a deterministic offline builder for static `neo-runnerd` and
  `neo-runner-probe` binaries plus the reviewed release manifest, seccomp,
  systemd and environment artifacts.
- Emit and verify a strict content-addressed bundle manifest covering exact
  relative paths, modes, sizes and SHA-256 values. Reject symlinks, extras,
  missing files, byte/mode drift and placeholder/unapproved production bundles.
- Keep source and derived artifacts separate. The builder never installs or
  modifies the host; production installation remains an explicit operator
  action on the approved target.
- Permit only loopback or literal private Runner bind addresses. Wildcard,
  hostname and public binds fail before listen. Add exact probe-before-start
  and Runner-owned state/runtime directory contracts to systemd.

### Stage-specific activation contract

- Add a closed `neo.agent-production-activation/v1` schema, held template
  fixture, strict read-only evaluator and offline negative/positive self-test.
- G21.0 supports only stage `control_plane`. Bind the exact release commit,
  migration head `090`, operations policy, Runner manifest/binary, target,
  endpoint, client certificate, server CA, service identities and review
  window.
- Require exact live checks for host isolation, clean-copy install, private
  mTLS probe, zero-inventory reconcile and rollback/Kill-Switch readiness.
- The authorization vector permits only control-plane maintenance and must
  explicitly deny Root Run, Broker read/mutable effects, Child, Cron and
  Learning. Require zero orphan/Scratch residue.
- Template, stale, incomplete, drifted, widened, non-production, failed/not-run
  or placeholder evidence never activates the worker.

### Dedicated production control worker

- Add a separate `mm-chat-agent-runtime-control` binary and internal package.
  Never import Runner production control into `cmd/api`.
- Require a dedicated PostgreSQL login inheriting only
  `agent_runner_control`; keep the API, Runner host and Broker credentials
  separate.
- On every cycle: revalidate activation and mounted hashes, probe required
  isolation features, list remote Sandboxes, load PostgreSQL recovery inventory,
  reconcile exact non-expired expected Sandboxes, then prove post-reconcile
  inventory equality.
- Startup/cycle drift exits nonzero. Healthcheck is read-only and proves the
  same activation/probe/inventory relation without issuing reconcile.
- Do not acquire a Step, sign launch authority, call launch/heartbeat/cancel,
  run Broker adapters or create user-facing Agent execution in G21.0.

### Configuration, Compose and preflight

- Add default-off Agent flags and exact Runner control settings to the example
  environment and Compose boundary. The current live environment is untouched.
- Add an opt-in `agent-runtime-control` Compose profile with no port, no
  Provider/object-store/MCP secret, read-only rootfs, all capabilities dropped
  and only the private application network.
- Production preflight validates booleans, forbids any execution-stage flag,
  requires a sixth distinct PostgreSQL principal, exact private HTTPS Runner
  URL, bounded timings, secure non-symlink evidence/mTLS files, certificate/key
  pairing and READY G21.0 evidence with exact hash bindings.
- Disabled defaults must not read files, dial a Runner or require target-host
  credentials. Exact-host failure remains `ISOLATION_UNAVAILABLE` here.

### Documentation and gates

- Add a focused G21.0 verifier covering schema/evaluator, bundle determinism and
  tamper cases, Go race/vet tests, default-off source wiring, Compose/preflight
  contracts and expected current-host hold.
- Integrate the focused gate into Phase 0/full standalone without treating an
  offline positive fixture as live evidence.
- Add a G21 tracking plan for G21.0-G21.6 and synchronize architecture,
  executable contract, deployment and Trellis specs.

## Acceptance criteria

- [x] A template deployment bundle builds reproducibly in a temporary directory
      and verifies; symlink, extra file, mode/hash drift and false production
      promotion are rejected.
- [x] Runner refuses root, wildcard/hostname/public bind and accepts only
      loopback/private literal bind; systemd performs exact pre-start probe.
- [x] Activation schema/evaluator prove READY only for a temporary exact
      production `control_plane` record and hold/reject template, stale,
      incomplete, widened, drifted and residue cases.
- [x] Control worker uses mTLS plus dedicated `agent_runner_control` repository,
      reconciles only expected recovery inventory and exposes no execution or
      public HTTP path.
- [x] Default Compose/example configuration keeps every Agent flag false and
      starts no control worker unless the explicit profile is selected.
- [x] Production preflight rejects shared DB principals, public/plaintext
      endpoints, insecure/mismatched files, non-READY evidence and any Root/
      Broker/Child/Cron/Learning enablement.
- [x] Focused, component and standalone gates pass; exact-host gate remains
      nonzero with `ISOLATION_UNAVAILABLE` on this development host.
- [x] Protected runtime paths and live environment remain untouched.

## Definition of done

- Source, tests, schemas, fixtures, scripts, Compose/example config, deployment
  artifacts, docs/tracking and Trellis specs agree on the control-only boundary.
- The work commit, task archive and journal commits are created without amend or
  push.

## Technical approach

Use one new `agentactivation` package for strict runtime evidence and binding
validation, and one `agentruntimecontrol` package for the control-only loop.
Use the existing `agentrunner.RPCClient` and `PostgresControlRepository`; add a
strict outbound request constructor and client-certificate identity binding.
Build the host bundle into an explicit derived directory and verify it with a
standard-library Python tool.

## Decision (ADR-lite)

**Context:** Wiring through `cmd/api` would violate the deliberately separate
`agent_runner_control` database boundary, while requiring the final G20.10
closure before first connection would create a canary/evidence cycle.

**Decision:** Introduce a dedicated control worker and a short-lived G21.0
stage activation record. It may probe and reconcile only; execution remains
physically absent and false.

**Consequences:** Exact-host deployment and mTLS maintenance become deployable
without unlocking Agent execution. G21.1 can add Root Run acquisition/launch on
top of this worker under a new, separately evidenced authorization stage.

## Out of scope

- Root Run acquisition/launch/cancel/heartbeat and user-visible execution.
- Real Broker/Project/vault/MCP adapters or any mutable external effect.
- Child Agent, Cron Scheduler, Learning worker, Shadow promotion or final
  `PROMOTION_READY` production closure.
- Migration `091`, live host installation, live credentials, live evidence or
  changes to protected runtime state.

## Research references

- [`research/runtime-wiring-boundary.md`](research/runtime-wiring-boundary.md)
- [`research/staged-activation-evidence.md`](research/staged-activation-evidence.md)
- [`research/exact-host-bundle.md`](research/exact-host-bundle.md)

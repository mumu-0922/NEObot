# G21.5 Draft check transport research

## Question

How can isolation/evaluation checks use the accepted rootless Runner path
without pretending an unadmitted Draft is an ordinary package Run?

## Repository findings

- `agentlearning.RunChecks` supports injected `Checker` implementations, but
  defaults isolation/evaluation to `CHECK_UNAVAILABLE`.
- Normal Runner lifecycle authority is bound to `agent_run_attempts` through
  migration 085. A Draft is not an admitted package and has no legitimate
  Orchestrator Run/Attempt.
- `neo-runnerd` already provides the required rootless Podman isolation:
  read-only rootfs, no capabilities, no network, private namespaces, seccomp,
  cgroup/resource limits, a read-only pre-staged Workspace, durable request
  replay and per-caller mTLS method policy.
- The Runner can receive bounded artifacts from a Sandbox through its Unix
  intake socket, but the control plane currently cannot read an exact artifact
  result before cancellation.
- G21.1-G21.4 synthetic canaries already use operator-pre-staged Workspace
  snapshots. G21.5 can do the same for one exact Draft; generic archive upload
  and product Draft selection are not required until a later stage.

## Comparable patterns

- Remote-execution systems distinguish an execution lease from package
  admission; a test action may run immutable input without publishing it.
- CI runners use ephemeral jobs with fixed images, read-only source mounts and
  result artifacts, while a separate protected-environment gate controls
  release.
- Supply-chain scanners bind attestations to immutable subject digests and
  evaluator/suite digests; a pass does not itself grant installation authority.

## Recommended protocol

1. Migration 094 creates durable Draft-check attempts keyed by activation,
   Draft fingerprint, check generation and kind (`isolation` or `evaluation`).
   The lease is not an `agent_run_attempt` and cannot reach Broker/Provider,
   Cron, delegation or package admission functions.
2. The worker loads a strict activation plan binding the exact Draft, proposed
   package/runtime/archive fingerprints, pre-staged Workspace ID/fingerprint,
   fixed checker image/argv, empty Tool Registry, no network/secrets and small
   resources.
3. A target-scoped SQL function validates the live Draft check claim and emits
   short-lived Runner authority claims for only launch/result/cancel. The Go
   worker signs those claims with the activation-bound key.
4. `neo-runnerd` adds a read-only `result` lifecycle RPC. It returns only one
   exact bounded quarantined artifact for the authenticated Attempt; it does
   not receive PostgreSQL, object-store or package-admission credentials.
5. The fixed checker writes a strict content-free result artifact through the
   existing Unix intake. The payload binds Draft/check generation/kind,
   proposed/archive/workspace/suite fingerprints, status, reason, duration and
   bounded integer metrics. The worker validates it, persists the result in
   migration 094, cancels/reaps the Sandbox, then publishes the three-receipt
   bundle through migration 089.
6. A crash can replay a persisted result or wait for lease expiry and reconcile
   the old Sandbox. It must never launch a second live attempt for the same
   activation/Draft/generation/kind.

## Alternatives rejected

- **Deterministic in-process fake:** useful in unit tests but not exact-host
  isolation/evaluation evidence.
- **Ordinary Agent Run:** grants an unadmitted Draft normal package execution
  semantics and pollutes product Run history.
- **Give Runner object-store credentials:** widens the host isolation boundary
  and lets a compromised Runner enumerate unrelated objects.
- **Generic archive upload RPC now:** unnecessary for the exact synthetic target
  and creates a large unaudited input surface before G21.6.

## Required proof

- The learning caller has only `probe/list/reconcile/launch/result/cancel`; no
  heartbeat, Prepare or Commit.
- Result is attempt/generation/name/fingerprint bounded, size-capped, replay-safe
  and inaccessible to every earlier caller.
- Draft authority cannot be rebound to another Draft, Workspace, image, argv,
  suite, package/runtime, check kind or generation.
- Crash before/after launch/result/persistence/cancel ends with one durable
  result and zero Runner residue.

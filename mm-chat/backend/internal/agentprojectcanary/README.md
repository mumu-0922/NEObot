# agentprojectcanary

`agentprojectcanary` owns the G21.3 one-action controller, strict plan bindings,
detached approval verification and private relay target for a synthetic Project
CAS. It has no HTTP/API registration and cannot select user Projects or
package-provided arguments.

## Responsibilities

- Freeze the only admitted Project Tool, action, resource, patch and Runtime
  identities before any durable work begins.
- Verify the detached operator approval after Prepare and append only its fixed
  `per_commit` authority to the matching immutable intent.
- Route authority-verified Project Prepare/Commit calls through the dedicated
  private relay without sharing Broker/Artifact credentials.
- Resolve acknowledgement loss from durable CAS status, terminalize ambiguity
  without retry and restore only the reviewed synthetic baseline.

## Main contracts

- `LoadPlan` and `NewBindings` derive one immutable Grant, Registry, Run, Step,
  snapshot, arguments and mutation fingerprint.
- `ActivationBindingFingerprint` produces the stable no-cycle activation
  identity.
- `LoadApproval` verifies strict JSON, secure files, Ed25519 signature, exact
  release/target/plan/request/action bindings and a maximum 15-minute window.
- `NewRelayTarget` resolves only authority-verified Project Prepare/Commit
  requests against the frozen bindings.
- `Service.Cycle` proves Runner readiness, launches one synthetic Sandbox,
  prepares, records approval, commits, terminalizes and restores the baseline.
- `Service.Health` is read-only and requires current activation plus empty
  Runner inventory.

## Usage

Construct this package only from the standalone G21.3 command after strict
activation, approval-file and database-role verification. Production
composition loads the frozen plan, derives bindings, registers the Project CAS
executor, creates the caller-specific relay target and then creates `Service`.
`cmd/api` must never import this package.

The production composition is in
[`cmd/agent-runtime-project-canary`](../../cmd/agent-runtime-project-canary/).
Its direct dependencies are the private `agentactivation`, `agentorchestrator`,
`agentrunner`, `agentbroker` and `agentruntimecontrol` contracts. Runner and
Sandbox remain credential-free; PostgreSQL and approval authority stay in the
standalone controller.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/agentprojectcanary
```

Cross-layer verification is owned by
`scripts/verify-agent-runtime-g21-3.sh`.

See [`DESIGN.md`](./DESIGN.md) for trust boundaries and failure semantics.

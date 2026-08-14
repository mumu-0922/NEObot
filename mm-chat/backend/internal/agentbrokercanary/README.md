# agentbrokercanary

`agentbrokercanary` owns the G21.2 synthetic Broker and Artifact canary. It
loads one immutable plan, derives stable Run/Step/Grant identities, drives five
independent Runs through the real Runner Prepare/Commit seam, and reconciles
every Sandbox to zero inventory.

## Responsibilities

- Require exactly one Project read, Workspace read, auth-none MCP read,
  Artifact publication and acknowledgement-loss action.
- Freeze Capability Grant, Tool Registry, snapshot and argument fingerprints
  for every action.
- Confine file reads with Linux `openat2`, `RESOLVE_BENEATH` and no-symlink
  resolution; return content-free receipts only.
- Restrict MCP execution to one manifest-defined, read-only, idempotent Tool
  with no credential environment.
- Publish bounded text/JSON Artifacts from private quarantine through the
  ordinary object store and PostgreSQL attachment authority.
- Exercise exact Prepare/Commit replay and terminal `outcome_unknown` without
  a second dispatch.

## Main API

| API | Purpose |
| --- | --- |
| `LoadPlan` / `NewBindings` | Strictly validate the private plan and derive immutable action authority. |
| `NewRelayTarget` | Resolve relayed Runner requests against plan-owned Grant and Registry state before calling Broker. |
| `NewExactFileReader` | Construct a bounded Project or Workspace reader. |
| `NewMCPReadExecutor` | Construct the reviewed auth-none MCP read executor. |
| `NewArtifactExecutor` | Construct the Artifact publication executor. |
| `NewDirectoryQuarantine` | Open the exact private quarantine root without following links. |
| `NewService` | Compose activation, Orchestrator, Runner authority and lifecycle recovery. |

## Usage

Construct this package only from the standalone G21.2 command after strict
activation and database-role verification. Production callers load the plan,
derive bindings, register the five reviewed executors, build the relay target
and then create `Service`; `cmd/api` must never import the package.

The production composition is in
[`cmd/agent-runtime-broker-canary`](../../cmd/agent-runtime-broker-canary/).
Package tests use synthetic fakes; exact-host promotion still requires the
separate isolation and activation gates.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/agentbrokercanary
```

See [`DESIGN.md`](./DESIGN.md) for trust boundaries and failure semantics.

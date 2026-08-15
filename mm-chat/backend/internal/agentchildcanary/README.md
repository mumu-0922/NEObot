# Agent Child Canary

`agentchildcanary` owns the independently activated G21.4 synthetic depth-one
Child Agent canary. It composes the existing Orchestrator, delegation and
Runner control contracts without exposing delegation to HTTP, Chat or Package
inputs.

## Responsibilities

- Load one strict synthetic Parent/Child plan from a private regular file.
- Create one depth-zero Parent with only `delegate_task/create` authority.
- Ask for `delegate_task` on the depth-one Child and prove the derived Tool
  Registry is physically empty.
- Launch both rootless, no-network Sandboxes through signed lifecycle-only
  Runner RPC.
- Cascade the Child first, wait out late launch authority, reap the exact Child,
  settle its reserved budget, and only then cancel the Parent.
- Resume monotonically after restart without creating a second Child lineage.

## Integration

Construct `Service` with the dedicated PostgreSQL repositories, an
`agentdelegation.Service` using `RunnerReaper`, and the dedicated Runner mTLS
client. The production entrypoint is
`cmd/agent-runtime-child-canary`; the profile remains disabled by default.

No API, Provider, Broker relay, MCP, Project, Artifact, Egress or Secret client
is imported by this package.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/agentchildcanary ./internal/agentdelegation
```

See [DESIGN.md](DESIGN.md) for restart and authority boundaries.

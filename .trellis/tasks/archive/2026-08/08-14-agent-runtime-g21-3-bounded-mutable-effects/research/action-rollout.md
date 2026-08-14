# G21.3 action rollout research

## Question

Which mutable action should be the first production-shaped G21.3 slice without
silently turning the Broker canary into generic Agent execution?

## Existing repo facts

- G21.2 already proves Runner-to-Broker Prepare/Commit, exact replay, Artifact
  publication and terminal `outcome_unknown`, but its strict plan rejects
  generic mutable actions and the global mutation flag remains false.
- `internal/agentbroker` already defines canonical Project patches, a
  `ProjectCASExecutor`, a mutable MCP adapter and durable migration `086`
  approval/Commit authority. The only Project CAS implementation is explicitly
  a deterministic test fake.
- No production user Project file store or mutable MCP Tool is currently wired.
  The development host remains `ISOLATION_UNAVAILABLE`.

## Compared slices

### A. One exact synthetic Project CAS action — recommended

Add an independent canary that can mutate only a pre-provisioned synthetic
Project resource through exact compare-and-swap authority. It exercises a real
mutable Commit, explicit approval, stable status and crash recovery without
network, Secret, object-store or user-facing authority.

This is the narrowest step from G21.2 because Project patch types and the Broker
Commit state machine already exist, while the missing status authority can be
made durable and independently testable.

### B. One exact MCP write Tool

A pinned MCP write/status pair could prove a remote mutable dispatch, but it
would introduce MCP credentials or auth-none write semantics, network/status
protocol assumptions and an external cleanup contract in the same release.
That expands three trust boundaries at once and conflicts with the G21.3 rule
that no generic MCP write capability is admitted.

### C. Generic mutable Broker/Egress/Secret activation

This would maximize short-term capability but erase the action-by-action
promotion boundary, require a vault and general Egress policy, and make one
canary identity a broad production executor. It is rejected.

## Recommended boundary

- Admit exactly `project.patch` / `project.write` / `apply_patch` for one
  synthetic resource, one UTF-8 file write, one frozen base revision and a
  small byte budget.
- Keep `AGENT_BROKER_MUTATION_ENABLED=false`; use a separate canary-specific
  flag so broad product mutation remains physically unavailable.
- Do not enable MCP write, external Egress, Secret handles, deletion, multi-file
  patches, user Projects, API/Chat entrypoints or package-driven arbitrary
  arguments.
- Treat source/disposable success only as held evidence. Exact-host promotion
  remains a separate operation.

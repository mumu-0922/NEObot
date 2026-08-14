# agent-runtime-project-canary command

`agent-runtime-project-canary` is the standalone G21.3 controller for one
offline-approved synthetic Project compare-and-swap mutation. It is separate
from the API, G21.0 control worker, G21.1 Root canary, G21.2 Broker/Artifact
canary and `neo-runnerd`.

## Commands

```text
agent-runtime-project-canary run
agent-runtime-project-canary healthcheck
```

`run` validates activation and approval, starts the private Project relay,
executes or reconciles the one frozen action, restores the reviewed baseline
and then remains in health mode. `healthcheck` revalidates activation, Runner
readiness and empty inventory without dispatching another mutation.

## Authority boundary

The command accepts only `project.patch/project.write/apply_patch`, one exact
synthetic resource, one flat UTF-8 path, one bounded content value and
`per_commit` approval. It verifies a detached Ed25519 approval using a public
key distinct from Runner authority and TLS keys. The approval private key is
never mounted.

The database LOGIN recursively inherits exactly:

- `agent_orchestrator_runtime`
- `agent_runner_control`
- `agent_effect_control`
- `agent_project_mutation_control`

It has no owner membership, provision EXECUTE or direct table DML. The process
receives no S3, MCP, Provider, vault or generic Egress credential. Runner and
Sandbox receive neither the database URL nor approval/key material.

## Default-off deployment

```bash
docker compose --env-file .env.single-server \
  --profile agent-runtime-project-canary \
  up -d agent-runtime-project-canary
```

The profile may start only with current G21.0-G21.2 readiness and
`AGENT_PROJECT_MUTATION_CANARY_ENABLED=true`; every broad Runtime/Broker
mutation/MCP write/Egress/Secret/Child/Cron/Skill-install/Learning switch stays
false.

## Verification

```bash
cd mm-chat/backend
go test ./cmd/agent-runtime-project-canary ./internal/agentprojectcanary
cd ..
bash scripts/verify-agent-runtime-g21-3.sh
```

See [`DESIGN.md`](./DESIGN.md) and
[`docs/deployment/agent-runtime.md`](../../../docs/deployment/agent-runtime.md).

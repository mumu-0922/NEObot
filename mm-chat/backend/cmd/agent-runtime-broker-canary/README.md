# agent-runtime-broker-canary command

`agent-runtime-broker-canary` is the standalone G21.2 process for reviewed
read-only Broker actions and bounded Artifact publication. It is separate from
the API, G21.0 control worker, G21.1 Root canary and `neo-runnerd`.

## Commands

```text
agent-runtime-broker-canary run
agent-runtime-broker-canary healthcheck
```

## Usage

`run` opens the private Runner-to-Broker relay and executes or observes the
five deterministic synthetic Runs. `healthcheck` performs the same activation,
role, Runner and object-store readiness checks without launching a new action.

Production uses the default-off profile:

```bash
docker compose --env-file .env.single-server \
  --profile agent-runtime-broker-canary up -d agent-runtime-broker-canary
```

## Startup boundary

Startup requires the dedicated feature flag while all broad Runtime flags stay
false, exact G21.2 activation evidence, matching Ed25519 keys, separate canary
and Runner-relay mTLS identities, a literal-private relay endpoint, an existing
S3-compatible bucket, and a PostgreSQL LOGIN inheriting exactly:

- `agent_orchestrator_runtime`
- `agent_runner_control`
- `agent_effect_control`
- `agent_artifact_control`

The LOGIN has function-only mutation authority and no direct table DML. The
process may hold Broker database, object-store and MCP Runner credentials;
`neo-runnerd` and Sandbox must not receive them.

## Verification

```bash
cd mm-chat/backend
go test ./cmd/agent-runtime-broker-canary
cd ..
bash scripts/verify-agent-runtime-g21-2.sh
```

See [`DESIGN.md`](./DESIGN.md) and the deployment contract under
[`docs/deployment/agent-runtime.md`](../../../docs/deployment/agent-runtime.md).

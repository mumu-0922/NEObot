# agent-runtime-root-canary command

`agent-runtime-root-canary` is the standalone G21.1 process entrypoint for one
synthetic Root Run. It wires the immutable canary plan, production activation
evidence, an independent PostgreSQL LOGIN, a dedicated mTLS identity and
Ed25519 launch authority into `internal/agentrootcanary`.

The command is deliberately separate from `cmd/api` and the G21.0 control
worker. It does not expose a port or enable user-triggered Agent execution.

## Commands

```text
agent-runtime-root-canary run
agent-runtime-root-canary healthcheck
```

- `run` continuously revalidates activation evidence and executes or recovers
  the single idempotent canary until shutdown.
- `healthcheck` performs the same fail-closed startup validation, then checks
  activation, the private Runner and zero canary inventory without launching a
  workload.

Production should select the default-off Compose profile instead of invoking
the binary directly:

```bash
docker compose --env-file .env.single-server \
  --profile agent-runtime-root-canary up -d agent-runtime-root-canary
```

All required environment variables and mounted-file contracts are documented
in [`docs/deployment/agent-runtime.md`](../../../docs/deployment/agent-runtime.md).

## Startup boundary

Before opening the worker loop, the command requires:

- `AGENT_ROOT_RUN_CANARY_ENABLED=true` while general Runtime, Broker, Child,
  Scheduler, Skill-install and Learning switches remain false;
- an exact `root_run_canary` production activation bound to the immutable plan
  and public authority key;
- the dedicated mTLS identity
  `spiffe://neo-chat/agent-runtime-root-canary`;
- matching Ed25519 private/public keys; and
- a PostgreSQL LOGIN whose recursive inherited roles are exactly
  `agent_orchestrator_runtime` and `agent_runner_control`.

Configuration and startup errors are sanitized before logging. Lease tokens,
database credentials, key material and evidence contents must never be logged.

## Verification

From `mm-chat/` run:

```bash
bash scripts/verify-agent-runtime-g21-1.sh
bash scripts/verify-agent-runtime-phase0.sh
```

Focused Go coverage lives in `main_test.go`. See [`DESIGN.md`](./DESIGN.md) for
the command wiring and rollback decisions.

# agentrootcanary

`agentrootcanary` composes the existing durable Orchestrator and Runner control
packages for the separately activated G21.1 synthetic Root Run canary. It
proves one narrow production execution path without exposing an API or
enabling general Agent Runtime, Broker, Child, Cron or Learning authority.

## Responsibilities

- load and strictly validate one immutable, content-free canary plan;
- idempotently enqueue and claim one depth-zero synthetic Run;
- issue request-bound launch, heartbeat and cancel authority;
- persist expected/running Runner Sandbox projection;
- exercise both Runner and PostgreSQL lease heartbeats;
- cooperatively cancel/reap the Sandbox and require zero remote inventory;
- atomically cancel Sandbox, Attempt, Step and Run projections; and
- fence restart recovery when the memory-only lease token has been lost.

## Dependencies

- `internal/agentorchestrator` owns Run/Step/Attempt leases and events.
- `internal/agentrunner` owns signed RPC authority, Sandbox projection and the
  private Runner client.
- PostgreSQL migrations `084` and `085` provide the only durable mutation
  functions. This package adds no migration.
- `cmd/agent-runtime-root-canary` supplies activation, mTLS, key, role and
  process lifecycle wiring.

## Usage

Construct the service only after validating the external `root_run_canary`
activation and loading the private plan and Ed25519 keys:

```go
service, err := agentrootcanary.NewService(
    config,
    plan,
    activationGate,
    agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db)),
    authority,
    agentrunner.NewPostgresControlRepository(db),
    runnerClient,
    agentrootcanary.NewPostgresTerminalRepository(db),
)
if err != nil {
    return err
}
_, err = service.Cycle(ctx)
```

Production uses the command rather than importing this package into
`cmd/api`. Run the focused source and PostgreSQL gates from `mm-chat/`:

```bash
bash scripts/verify-agent-runtime-g21-1.sh
bash scripts/verify-agent-root-canary-postgres17.sh
```

## Public API

| API | Purpose |
| --- | --- |
| `LoadPlan` / `ValidatePlan` | Read a secure regular file and enforce the immutable synthetic plan. |
| `NewService` | Compose activation, Orchestrator, authority, Runner and terminal dependencies. |
| `Service.Cycle` | Revalidate, recover or execute the one canary to terminal cleanup. |
| `Service.Health` | Read-only activation, Runner probe and zero-inventory check. |
| `NewPostgresTerminalRepository` | Build the atomic Sandbox/Run terminal repository. |
| `PostgresTerminalRepository.FinalizeCanceled` | Commit all four canceled projections and three events in one transaction. |

## Directory

```text
agentrootcanary/
├── DESIGN.md
├── README.md
├── plan.go
├── repository_postgres.go
├── repository_postgres_test.go
├── service.go
└── service_test.go
```

See [`DESIGN.md`](./DESIGN.md) and
[`docs/contracts/agent-runtime.md`](../../../docs/contracts/agent-runtime.md).

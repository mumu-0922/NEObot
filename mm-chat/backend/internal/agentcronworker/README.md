# agentcronworker

`agentcronworker` owns the G21.5 polling loop for one exact, operator-bound
Cron activation target. PostgreSQL migration `094` provides the cohort boundary;
this package never claims globally and filters in Go.

## Responsibilities

- reconcile expired target claims before acquiring new work;
- invoke the existing `agentcron.Service` scheduling/enqueue semantics;
- prune only through the activation-scoped repository on a bounded cadence;
- stop on database or scheduling errors instead of broadening authority.

It does not create or edit Templates, enable the generic Scheduler, execute
Runs, connect to the Runner, or receive object-store/Provider credentials.

## Usage

```go
repository := agentcron.NewActivationPostgresRepository(db, activationID)
scheduler := agentcron.NewService(repository)
worker, err := agentcronworker.NewService(agentcronworker.Config{
    Owner: "agent-cron-worker", PollInterval: time.Second,
    LeaseDuration: 30 * time.Second, BatchSize: 100,
    Retention: 30 * 24 * time.Hour, MaintenanceEvery: 60,
}, scheduler)
if err != nil { return err }
return worker.Run(ctx)
```

## API

- `NewService`: validates bounded worker configuration.
- `RunOnce`: reconcile, schedule, and optionally prune one cycle.
- `Run`: repeats `RunOnce` until cancellation or a fail-closed error.

See [DESIGN.md](DESIGN.md) for authority and failure-ordering details.

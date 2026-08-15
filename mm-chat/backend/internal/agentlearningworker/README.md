# agentlearningworker

`agentlearningworker` implements the G21.5 rootless isolation/evaluation
checkers for one exact quarantined Skill Draft. It uses a distinct lifecycle-
only Runner caller and migration-094 Draft-check attempts; it never creates an
ordinary Agent Run or obtains Promote authority.

## Responsibilities

- load and validate a strict synthetic plan from a private regular file;
- create attempt/generation-fenced Draft-only Runner checks;
- issue short-lived `launch`, `result`, and `cancel` authority;
- verify the exact bounded JSON artifact and persist its SHA-256 receipt;
- cancel/reap the Sandbox before returning a check receipt.

## Usage

```go
plan, err := agentlearningworker.LoadPlan(planFile)
repository := agentlearningworker.NewPostgresRepository(db, plan.ActivationID)
authority, err := agentrunner.NewControlService(repository, privateKey)
runtime, err := agentlearningworker.NewRuntime(plan, repository, authority, runnerClient)

learning := agentlearning.NewService(
    agentlearning.WithRepository(
        agentlearning.NewActivationPostgresRepository(db, plan.ActivationID)),
    agentlearning.WithObjectStore(objects),
    agentlearning.WithCheckers(agentlearning.StaticChecker{},
        runtime.IsolationChecker(), runtime.EvaluationChecker()),
    agentlearning.WithLearningEnabled(true),
)
```

The enablement above belongs only in the dedicated G21.5 command. The broad API
process continues to construct learning with `false`.

## API

- `LoadPlan` / `ValidatePlan`: strict plan and Sandbox boundary.
- `NewPostgresRepository`: migration-094 attempt/request/result persistence.
- `NewRuntime`: constructs isolation and evaluation `agentlearning.Checker`s.

See [DESIGN.md](DESIGN.md) for lifecycle and trust boundaries.

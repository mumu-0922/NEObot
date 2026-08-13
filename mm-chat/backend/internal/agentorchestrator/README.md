# agentorchestrator
`agentorchestrator` is the G20.2 internal durable control-plane seam. It owns
canonical immutable Run snapshots, idempotent Run/Step enqueue, legal
Run/Step/Attempt transitions, generation-fenced leases, recovery inventory,
event projection rebuild, hierarchical Kill Switch resolution and terminal
retention.

It has no HTTP handler, process startup wiring, Redis dependency, Runner RPC,
Sandbox launch, Tool grant, side-effect broker, Child Agent, Cron or learning
path. Importing this package does not make Agent execution available.

## Verification

```bash
go test -race ./internal/agentorchestrator
bash ../scripts/verify-agent-orchestrator.sh
bash ../scripts/verify-agent-orchestrator-postgres17.sh
```

See [DESIGN.md](DESIGN.md) for the authority and failure boundaries.

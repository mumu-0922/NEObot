# memoryworker

`memoryworker` is the private Go consumer for durable Memory capture in server
mode. PostgreSQL outbox/job rows are authoritative; Redis only reduces polling
latency.

## Responsibilities

- claim bounded jobs through lease-fenced PostgreSQL functions;
- hydrate only the job's current user message and Server-owned provider profile;
- run the existing bounded Memory extraction semantics with a hard timeout;
- validate, secret-filter, and apply at most five candidates through the
  existing `usermemory.Service` contract;
- retry transient failures, dead-letter terminal drift, and resume expired
  leases after crashes or rolling restarts;
- report readiness without exposing an HTTP port.

The package never accepts browser identity, never reads queue/source tables
directly, and never treats Redis as durable state.

## Runtime

The command is built into the backend image:

```bash
/usr/local/bin/mm-chat-memory-worker run
/usr/local/bin/mm-chat-memory-worker healthcheck
```

`cmd/memory-worker` supplies the dedicated database URL, provider vault,
bounded pool/concurrency settings, and optional Redis subscription. See
[`docs/deployment/single-server-compose.md`](../../../docs/deployment/single-server-compose.md)
for the complete environment and role-provisioning contract.

## Main boundaries

| Boundary | Purpose |
| --- | --- |
| `New(Repository, ProviderResolver, ...Option)` | Validate and construct the bounded worker. |
| `Worker.Run(ctx, wake)` | Poll PostgreSQL continuously and consume optional wake hints. |
| `Worker.ProcessOne(ctx)` | Claim and finish one lease-fenced job. |
| `NewPostgresRepository(*sql.DB)` | Call only migration `054` worker capabilities. |
| `NewStoredProviderResolver(...)` | Reuse Server provider/vault activation rules. |

## Files

```text
worker.go                polling, retry, schema/stage dispatch, and apply flow
repository_postgres.go   restricted migration-054 function calls
provider.go              hydrated Server provider resolution
extraction.go            bounded prompt, parsing, validation, and secret filter
types.go                 job, capture, readiness, and interface contracts
*_test.go                offline worker, extraction, and failure-path coverage
```

See [DESIGN.md](DESIGN.md) for authority, failure, and security decisions.

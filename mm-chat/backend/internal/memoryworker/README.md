# memoryworker

`memoryworker` is the private Go consumer for durable Memory capture in server
mode. PostgreSQL outbox/job rows are authoritative; Redis only reduces polling
latency.

## Responsibilities

- claim bounded jobs through lease-fenced PostgreSQL functions;
- hydrate at most eight current Conversation messages, ten bounded current
  Memory context rows, and the Server-owned provider profile;
- redact concrete secrets and disabled Sensitive segments before Provider
  egress, then strictly decode versioned extraction/decision JSON;
- atomically persist at most five hash-pinned shadow/Review/rejected proposals
  without changing canonical Memory;
- dispatch `purge` jobs before Provider hydration and idempotently wipe deleted
  canonical/revision/evidence plaintext through migration `055`;
- dispatch `review_expire` before Provider hydration and idempotently wipe
  candidate/normalized/tag/key plaintext after the fixed 30-day window;
- dispatch migration `062` L2 Scene purge, same-scope refresh, and derived
  BGE-M3 embedding lanes; purge remains provider-free when Scene shadow is off;
- dispatch migration `063` L3 Persona purge, Global stable-L1 refresh, and
  derived BGE-M3 embedding lanes; purge remains provider-free when Persona
  shadow is off;
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
| `NewPostgresRepository(*sql.DB)` | Call only migration `054`–`063` worker capabilities. |
| `NewStoredProviderResolver(...)` | Reuse Server provider/vault activation rules. |

## Files

```text
worker.go                polling, retry, schema/stage dispatch, and proposal flow
repository_postgres.go   restricted migration-054/055/056 function calls
provider.go              hydrated Server provider resolution
extraction.go            bounded versioned extraction/decision Provider calls
proposal.go              proposal normalization and scope/time/evidence validation
scene.go                 Scene lease, synthesis, member, and derived-embedding contracts
scene_synthesis.go       strict bounded Scene Provider proposal validation
scene_worker.go          provider-free purge plus default-off refresh/embedding dispatch
scene_repository_postgres.go restricted migration-062 Scene capabilities
persona.go               Persona lease, member, profile, and embedding contracts
persona_synthesis.go     strict bounded Persona Provider proposal validation
persona_worker.go        provider-free purge plus default-off refresh/embedding dispatch
persona_repository_postgres.go restricted migration-063 Persona capabilities
provider_privacy.go       pre-egress bounds plus shared usermemory redaction
strict_json.go            adapter to the shared internal/strictjson decoder
types.go                 job, capture, readiness, and interface contracts
*_test.go                offline worker, extraction, and failure-path coverage
```

See [DESIGN.md](DESIGN.md) for authority, failure, and security decisions.
The shared strict decoder is documented in
[`internal/strictjson`](../strictjson/README.md).

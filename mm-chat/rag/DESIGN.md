# Phase 15.2B Durable Worker Design

## Scope

This package implements durable orchestration, not RAG functionality. The
authority chain is:

```text
Go authority transaction
  -> knowledge_outbox / generation-bound processing job
  -> PostgreSQL SECURITY DEFINER state-machine functions
  -> Python poll/plan/heartbeat/retry/replay orchestration
```

Python treats function results as leased snapshots, never as permission to
invent ACL, consent, governance, generation, or projection state.

## Threat model and trust boundaries

- Outbox rows, job rows, Redis messages, handler failures, and database error
  text are untrusted. Only validated identifiers, allowlisted stages, canonical
  plans, and stable error codes cross orchestration boundaries.
- PostgreSQL functions are the sole mutation authority. The worker credential
  must have function execution grants only; compromise of the Python process
  must not permit direct authoritative-table DML or DDL.
- Redis may be lost, delayed, duplicated, forged, or flushed. A message carries
  no event identity or authority and can only accelerate the next Postgres scan.
- DSNs and replay credentials are secret input. They are never emitted in logs,
  metrics, readiness responses, CLI output, or exception text.
- Replay is a privileged operator boundary with a separate DSN, exact failed
  identity/error CAS, explicit execution flag, and durable database audit.
- Known residual risk: this package cannot enforce database GRANTs or container
  egress policy. Migration permission tests and deployment network policy remain
  mandatory promotion gates.

## Process topology

One process owns:

1. A dedicated PostgreSQL session holding one fixed advisory lock.
2. An async psycopg pool (`min=1`, `max=2`) for short function transactions.
3. A one-second outbox/job poll loop and 30-second exhaustive rescan trigger.
4. An optional Redis Pub/Sub subscriber that only sets an in-memory wake event.
5. A private Starlette/Uvicorn health and Prometheus listener.

The dedicated lock connection is outside the size-two transaction pool because
session locks require stable connection affinity. A second process fails startup
before any claim.

The build and runtime stages both use the absolute virtual-environment path
`/app/.venv`. Console-script shebangs therefore remain valid after the venv is
copied into the final image. `tests/docker-smoke.sh` guards this invariant by
starting `rag-worker` against a disposable readiness-function fixture and by
executing `rag-replay --help` from the final non-root image. The worker smoke also
uses a read-only root filesystem and the production capability/resource limits.

## Dark-run and promotion gate

`RAG_WORKER_DISPATCH_ENABLED=false` is the default. In that state no claim
function is called. If enabled, startup rejects an empty event registry and any
configured job stage lacking a handler. The checked-in production registries
are empty, so Phase B cannot claim parse, passage embedding, or purge work even
under accidental configuration drift. Tests inject synthetic registries without
changing production defaults.

## Outbox algorithm

- Every claim receives a new UUID lock token; one token is never reused.
- Normal poll/wake handles one row. Every 30 seconds, forced rescan drains up to
  a bounded safety limit directly from PostgreSQL pending/expired state. No
  local checkpoint or `MAX(id)` assumption exists, so late-low-ID commits remain
  visible; the applied ledger fences idempotency and result-hash conflicts in
  the atomic Apply/Ack transaction.
- Planning happens outside the claim transaction and performs no provider or
  object I/O.
- A versioned canonical JSON envelope is SHA-256 hashed deterministically.
- Apply + applied-event ledger + ack is one database function transaction.
- Invalid/unknown plans retry, then enter PostgreSQL `status=failed` DLQ at the
  bounded attempt limit. Only stable uppercase error codes cross the boundary.

## Job algorithm

- Global concurrency is one. The claim function receives only configured stage
  allowlists and excludes legacy unbound work at the database contract.
- A fresh lease token fences every claim. Heartbeat runs every 30 seconds for a
  90-second lease.
- Lease loss cancels local execution and never attempts finish with a stale
  token. Success/retry/fail all use the frozen finish function CAS.
- SIGTERM stops new claims. Current work has the configured grace period; after
  it expires local work is cancelled and its database lease is left to expire.

## Health semantics

`/health` only proves process/event-loop liveness. `/ready` reports fixed fields:
database, frozen functions, worker lock, consumer, projection, and Redis.
Projection `not_ready` is legal before `011`; Redis `degraded` never stops polling.
Only `/health` should be used for container restart policy.

## Replay and rollback

Replay is operator-only and defaults to dry-run. Execution requires exact ID,
expected error code, operator UUID, and reason. Job replay creates a caller-chosen
successor ID and does not revive the failed lease. Rollback is stopping the
worker process/profile; no schema downgrade or state mutation is performed by
this package.

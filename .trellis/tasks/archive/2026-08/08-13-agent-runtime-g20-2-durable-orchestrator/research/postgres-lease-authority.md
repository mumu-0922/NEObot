# PostgreSQL Lease Authority Research
## Evidence inspected

- `mm-chat/backend/migrations/054_memory_outbox_jobs_worker.up.sql`
- `mm-chat/backend/internal/memoryworker/repository_postgres.go`
- `.trellis/spec/backend/memory-v2-worker.md`
- `mm-chat/docs/contracts/agent-runtime.md`

## Comparable patterns

1. Neo Memory jobs use PostgreSQL `FOR UPDATE SKIP LOCKED`, exact worker/token,
   bounded TTL, `SECURITY DEFINER` functions and function-only runtime writes.
2. Durable workflow systems use monotonic attempt/run generations so an old
   worker cannot regain authority after timeout even if its delayed message is
   otherwise well formed.
3. Transactional outbox/event-sourcing patterns append the event and update the
   current projection in the same transaction; caches or wake queues contain
   IDs only and are disposable.

## Decision for G20.2

- PostgreSQL is the only authority. Lease acquisition locks the Run/Step,
  expires any stale predecessor, increments the Step generation, creates a new
  Attempt ID and appends all corresponding events atomically.
- Heartbeat and Attempt transition require exact Attempt, generation,
  lease-owner and opaque lease token, and require the lease to still be live.
- The runtime role receives no table DML. It can invoke narrowly scoped
  `SECURITY DEFINER` functions; a separate owner role owns tables/functions.
- Redis integration is intentionally absent. A future ID-only hint can be lost
  without affecting authorization, reclaim or restart recovery.
- Reclaim always appends `lease.expired` before the replacement
  `lease.acquired`; stale generations cannot publish a later transition.

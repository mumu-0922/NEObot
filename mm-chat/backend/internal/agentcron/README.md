# Agent Cron

`agentcron` is the held G20.6 scheduling control foundation. It validates and
fingerprints immutable approved Cron template revisions, calculates bounded
occurrences with explicit IANA timezone semantics, and coordinates durable
cursor and trigger claims through PostgreSQL.

The package can create a normal Orchestrator Run only through migration `088`'s
atomic occurrence-to-Run function. Every enqueue repeats current template,
owner, Skill, Grant, approval, expiry, Kill Switch, and overlap checks before
linking the occurrence to the stable Run identity.

It is intentionally not imported by HTTP, Chat, frontend, Compose, or a startup
worker. Passing these gates proves source and disposable-database behavior; it
does not enable the production Scheduler or satisfy exact-host isolation.

## Contracts

- Accept exactly standard five-field Cron expressions; descriptors, seconds,
  and embedded `TZ=` / `CRON_TZ=` prefixes are invalid.
- Resolve an independently frozen IANA timezone using Go's embedded `tzdata`.
- Store occurrence identity as `template + revision + scheduled UTC instant`.
- Bound missed work with `skip`, `fire_once`, or `catch_up`; bound overlap with
  `skip`, `buffer_one`, or `allow`.
- Retry only an occurrence not proven enqueued. A successful enqueue retains
  the same Run and idempotency identity across acknowledgement loss.
- Pause without materialization, resume beyond the current instant without
  backfill, and preserve tombstoned/history facts until bounded cleanup.
- Persist references and fingerprints only. Prompt bodies, Secret values,
  Workspace bytes, Tool arguments/results, and credentials are forbidden.

## Main API

`NewService` accepts a `Repository`. `NewPostgresRepository` binds that interface
to migration `088`'s narrow `SECURITY DEFINER` functions.

- `CreateRevision` / `GetTemplate`: create or read an immutable approved
  revision.
- `SetLifecycle` / `Resume`: pause, resume, or tombstone a template.
- `RevokeApproval`: append an automation-approval revocation.
- `RunCycle`: claim due cursors, materialize bounded occurrences, claim
  triggers, and enqueue or durably deny them.
- `Reconcile`: recover expired claims and terminalize exhausted retries.
- `Prune`: perform bounded retention cleanup, including eligible tombstones.

Held control-plane construction is deliberately explicit:

```go
repository := agentcron.NewPostgresRepository(database)
service := agentcron.NewService(repository)
```

Only a future private Scheduler may call the service after its own startup and
promotion gates pass. Do not import it into an HTTP handler or application
startup path in G20.6.

## Verification

```bash
bash mm-chat/scripts/verify-agent-cron.sh
bash mm-chat/scripts/verify-agent-cron-postgres17.sh
```

See [DESIGN.md](DESIGN.md) for the authority split, claim fencing, and held
promotion boundary.

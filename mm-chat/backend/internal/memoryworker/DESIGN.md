# memoryworker Design

## Goals

- Make completed-turn Memory extraction durable without increasing answer
  latency or making Redis authoritative.
- Recover safely from Provider failures, process crashes, lease expiry, and
  rolling restarts.
- Reuse existing Server provider/vault and `usermemory` validation semantics.
- Restrict the worker to one leased user's source and candidate-apply path.

## Non-goals

- Switching the v1 Memory reader or HTTP CRUD contract.
- Project/Conversation routing, semantic review/conflict handling, embeddings,
  L2/L3 summaries, or external Memory engines.
- Supporting request-only BYOK after the request ends; capture fails closed
  unless a current Server-owned provider profile can be hydrated.

## Architecture

```text
assistant finalize transaction
  -> ID-only memory_outbox + memory_jobs
  -> best-effort Redis event_id wake

memory-worker
  -> PostgreSQL claim + lease token
  -> extract: source/profile/generation/epoch-fenced hydration
       -> Server provider vault -> bounded extraction
       -> lease/tombstone-fenced Global candidate apply + evidence/revision
  -> purge: no Provider hydration -> tombstone/epoch-fenced plaintext wipe
  -> complete or bounded retry/dead-letter
```

Migration `054` owns the capability boundary. The runtime login inherits only
`memory_worker_runtime`, which can execute the hardened worker functions but
has no direct table access. Those functions execute as the restricted
`memory_runtime_owner` and pin `search_path` to the application schema,
`pg_catalog`, and `pg_temp`.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| PostgreSQL polling is authoritative | Redis delivery and connectivity are not durable | Redis-down only increases claim latency. |
| Same backend code/image, separate command | Reuse contracts without coupling worker failure to API availability | Worker gets its own login, pool, healthcheck, and resource limits. |
| Current and previous event schema majors only | Bound rolling-upgrade compatibility | Unknown majors dead-letter before hydration. |
| Lease token on hydrate/apply/complete/retry | Stale processes must lose write authority | Expired or reclaimed attempts cannot finish a job. |
| Profile/source/generation hashes are pinned | Provider, message, scope, and policy drift must fail closed | Changed state requires a new authoritative event, not stale replay. |
| Reuse `usermemory.Service.StoreExtracted` | Keep candidate normalization and limits consistent with v1 | PR3 retains Global v1 upsert semantics until the review/revision PR. |
| Dispatch purge before hydration | Deletion cleanup must not load or call a Provider | Purge remains available during Provider outages. |
| Recheck epoch and targeted tombstones at apply | A response returned after deletion has no write authority | Stale/tombstoned work dead-letters without resurrection. |
| Log IDs and error codes only | Source text, secrets, and raw Provider errors are private | Operators diagnose by bounded codes and queue state. |

## Failure and replay contract

| Condition | Result |
| --- | --- |
| Redis unavailable | Continue PostgreSQL polling. |
| Provider timeout/failure | Retry with bounded exponential backoff. |
| Unknown schema/stage or source/profile/epoch/tombstone drift | Dead-letter without candidate apply. |
| Worker crash during a lease | Reclaim after expiry while attempts remain. |
| Crash on final attempt | PostgreSQL marks `LEASE_EXPIRED` dead-letter. |
| Stale worker applies/completes | SQL rejects the old worker/lease token. |
| Duplicate event/job | Unique event/stage/idempotency keys return the first authority. |

Candidate application is per item rather than one candidates-wide transaction.
Replay safety comes from source/profile/epoch/tombstone fences, exact Global
upsert, append-only revision snapshots, and ID/hash-only evidence. Candidate-wide
Review proposals and semantic conflict routing belong to PR5.

## Threat model and controls

| Threat | Control |
| --- | --- |
| Cross-user hydration | Job user, source message, assistant parent, and Conversation ownership are joined and rechecked in SQL. |
| Stale response resurrects deleted/changed source | Active source, generation, Learn policy, profile timestamp/hash, and live lease are rechecked before every apply. |
| Purge sends deleted data to a Provider | Stage dispatch invokes the purge capability before any hydration or Provider resolution. |
| Prompt injection in source text | Source is JSON data under a Server-owned extraction prompt; output is schema-normalized and bounded. |
| Credential retention | Prompt prohibition plus content/tag credential-pattern rejection; event/job/log payloads contain no plaintext secret. |
| Compromised worker reads arbitrary tables | Runtime role has function execution only; owner membership and schema creation are forbidden. |
| Queue loss during Redis/worker outage | PostgreSQL rows remain pending and reclaimable. |

## Verification

- focused race tests for worker, chat capture, Redis wake, provider factory,
  runtime config, migration, and command packages;
- full backend tests and `go vet ./...`;
- disposable PostgreSQL migration up/down/re-up with atomicity, duplicate,
  stale lease, crash reclaim, cross-user denial, final-attempt dead-letter,
  candidate apply, and direct-table denial proofs;
- preflight/Compose rendering and backend-image build.

## Change history

- 2026-07-28: Memory v2 PR3 durable outbox/jobs and private Go worker.
- 2026-07-28: Memory v2 PR4 evidence/revision-aware apply, epoch/tombstone
  no-resurrection fences, and provider-free online purge.

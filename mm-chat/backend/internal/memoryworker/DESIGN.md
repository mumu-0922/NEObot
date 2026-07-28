# memoryworker Design

## Goals

- Make completed-turn Memory extraction durable without increasing answer
  latency or making Redis authoritative.
- Recover safely from Provider failures, process crashes, lease expiry, and
  rolling restarts.
- Reuse existing Server provider/vault and canonical `usermemory` validation
  semantics without giving model output canonical write authority.
- Restrict the worker to one leased user's source and atomic proposal path.

## Non-goals

- Switching the v1 Memory reader or HTTP CRUD contract.
- Review decisions/API/UI, canonical auto-apply, hybrid retrieval, embeddings,
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
  -> extract: source/profile/generation/epoch-fenced v2 hydration
       -> local secret/Sensitive redaction
       -> strict bounded extraction + conflict proposal
       -> one atomic shadow/Review/rejected candidate batch
  -> purge: no Provider hydration -> tombstone/epoch-fenced plaintext wipe
  -> review_expire: no Provider hydration -> day-30 proposal plaintext wipe
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
| Lease token on hydrate/propose/complete/retry | Stale processes must lose write authority | Expired or reclaimed attempts cannot finish a job. |
| Profile/source/generation hashes are pinned | Provider, message, scope, and policy drift must fail closed | Changed state requires a new authoritative event, not stale replay. |
| Reuse `NormalizeCandidateForStorage` only | Keep canonical normalization/limits without calling the legacy write path | PR5 cannot create or update canonical Memory. |
| Candidate-wide hash-pinned proposal | Partial Provider output and nondeterministic replay must not become authority | A committed batch resumes without another Provider call. |
| Separate versioned extraction and decision prompts | Extraction and relation classification have different duties | Both profile hashes are retained on every proposal. |
| Dispatch purge/review expiry before hydration | Cleanup must not load or call a Provider | Both lanes remain available during Provider outages. |
| Recheck epoch and targeted tombstones at proposal | A response returned after deletion has no proposal authority | Stale source work dead-letters; content/fact tombstones become hash-only rejection. |
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

Candidate persistence is one transaction per extraction output. The batch hash,
profile IDs, source/epoch/scope fences, normalized target revisions, and
`proposal_committed` resume flag prevent partial or divergent replay. Every PR5
outcome stays outside canonical Memory and active readers.

## Threat model and controls

| Threat | Control |
| --- | --- |
| Cross-user hydration | Job user, source message, assistant parent, and Conversation ownership are joined and rechecked in SQL. |
| Stale response survives a deleted/changed source | Active source, generation, Learn policy, profile timestamp/hash, and live lease are rechecked before proposal commit. |
| Purge sends deleted data to a Provider | Stage dispatch invokes the purge capability before any hydration or Provider resolution. |
| Prompt injection in source text | Messages and current Memory are JSON data under separate Server-owned prompts; duplicate/unknown/trailing output is rejected. |
| Credential retention/egress | Local sentence redaction precedes Provider calls; a secret proposal reaches SQL hash-only and logs remain code/ID-only. |
| Model spoofs authority or scope | Go restricts IDs to hydrated messages/targets and binds current Project/Conversation; SQL rechecks ownership and revision. |
| Review plaintext survives forever | A provider-free 128-attempt job expires/wipes shadow and pending proposals at day 30. |
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
- 2026-07-28: Memory v2 PR5 strict candidate/decision proposals, Review shadow,
  canonical auto-apply revocation, and provider-free 30-day expiry.

# memoryworker Design

## Goals

- Make completed-turn Memory extraction durable without increasing answer
  latency or making Redis authoritative.
- Recover safely from Provider failures, process crashes, lease expiry, and
  rolling restarts.
- Reuse existing Server provider/vault and canonical `usermemory` validation
  semantics without giving model output canonical write authority.
- Restrict the worker to one leased user's source, atomic proposal path, and
  narrow governance-backed safe-add promotion capability.

## Non-goals

- Switching the v1 Memory reader or HTTP CRUD contract.
- External Memory engines or any automatic reader promotion. Candidate
  auto-promotion in migrations `066`–`069` changes canonical storage only; it does
  not activate a reader or bypass prompt-injection gates.
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
       -> exact required candidate + decision Tool Calls
       -> one atomic shadow/Review/rejected candidate batch
       -> migration-069 compatible-profile current-authority safe-add promotion
  -> purge: no Provider hydration -> tombstone/epoch-fenced plaintext wipe
  -> review_expire: no Provider hydration -> day-30 proposal plaintext wipe
  -> l2_scene_purge: no Provider hydration -> stale plaintext/member/projection wipe
  -> l2_scene_refresh: same-scope current L1 -> strict derived Scene proposal
  -> l2_scene_embedding: current Scene -> fixed SiliconFlow BGE-M3 projection
  -> l3_persona_purge: no Provider hydration -> stale plaintext/member/projection wipe
  -> l3_persona_refresh: current stable Global L1 -> strict derived Persona proposal
  -> l3_persona_embedding: current Persona -> fixed SiliconFlow BGE-M3 projection
  -> complete or bounded retry/dead-letter
```

Migrations `054` and `066`–`069` own the capture capability boundary. The
runtime login inherits only `memory_worker_runtime`, which can execute the
hardened worker functions but has no direct table access. Those functions
execute as the restricted `memory_runtime_owner` and pin `search_path` to the
application schema, `pg_catalog`, and `pg_temp`.

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
| Separate required Tool Calls for extraction and decision | Free-text JSON and synthesized call identity cannot become write authority | One exact Provider-issued call, strict arguments, and both Tool profile hashes are required. |
| Reuse governance acceptance for safe adds | Canonical/evidence/audit logic must have one owner | Only current normal confirmed `SHADOW_ADD` candidates without tombstone, target, exact, or fact conflict become canonical. |
| Dispatch purge/review expiry before hydration | Cleanup must not load or call a Provider | Both lanes remain available during Provider outages. |
| Recheck epoch and targeted tombstones at proposal | A response returned after deletion has no proposal authority | Stale source work dead-letters; content/fact tombstones become hash-only rejection. |
| Log IDs and error codes only | Source text, secrets, and raw Provider errors are private | Operators diagnose by bounded codes and queue state. |
| Share strict JSON and privacy primitives | Worker and direct-action planners must not drift on duplicate keys or credential patterns | `internal/strictjson` and `usermemory` own the common fail-closed primitives. |
| L2 is derived and same-scope only | Conversation facts and model output cannot become broader authority | Refresh accepts only current Global or one Project member set and SQL recomputes every fence. |
| Scene purge ignores the rollout flag | Disabling Provider work must not weaken deletion | Stale Scene plaintext, members, and projections are still wiped within 24 hours. |
| L3 is derived from stable Global L1 only | Project/Conversation context and unstable types must not become user-wide identity | Refresh accepts only current `fact|preference|instruction|warning|decision` members and SQL recomputes every fence. |
| Persona purge ignores the rollout flag | Disabling Provider work must not weaken deletion or Sensitive-policy changes | Stale Persona plaintext, members, and projections are still wiped within 24 hours. |

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
| Crash after proposal or promotion | Replay skips Provider work and the accepted suggestion/decision audit prevents a second canonical row. |
| Candidate/evidence/Tool profile drift | Reject or retain Review; never auto-write stale content. |
| Scene member/watermark/generation drift | Reject the entire refresh or embedding result; no partial Scene survives. |
| Persona member/watermark/generation drift | Reject the entire refresh or embedding result; no partial Persona survives. |

Candidate persistence is one transaction per extraction output. Eligible
promotion is a second single SQL transaction that reuses governance acceptance.
The batch hash,
profile IDs, source/epoch/scope fences, normalized target revisions, and
`proposal_committed` resume flag prevent partial or divergent replay. Review,
conflict, tombstone, temporary, Sensitive-disabled, and secret outcomes stay
outside canonical Memory and every active reader.

## Threat model and controls

| Threat | Control |
| --- | --- |
| Cross-user hydration | Job user, source message, assistant parent, and Conversation ownership are joined and rechecked in SQL. |
| Stale response survives a deleted/changed source | Active source, generation, Learn policy, profile timestamp/hash, and live lease are rechecked before proposal commit. |
| Purge sends deleted data to a Provider | Stage dispatch invokes the purge capability before any hydration or Provider resolution. |
| Prompt injection in source text | Messages and current Memory are JSON data under separate Server-owned prompts; duplicate/unknown/trailing output is rejected. |
| Provider prose or adapter-generated Tool identity becomes authority | Extraction accepts only one exact required Tool Call and rejects missing, duplicate, failed, wrong-name, oversized, malformed, or synthetic-ID calls. |
| Persona widens authority | SQL hydrates only current stable Global L1 and Go accepts only a strict hydrated member subset; Provider output cannot add IDs or downgrade sensitivity. |
| Credential retention/egress | Local sentence redaction precedes Provider calls; a secret proposal reaches SQL hash-only and logs remain code/ID-only. |
| Model spoofs authority or scope | Go restricts IDs to hydrated messages/targets and binds current Project/Conversation; SQL rechecks ownership and revision. |
| Stale evidence or old free-text profile is promoted | Migrations `067`–`069` rehash every evidence message, enumerate evidence IDs by role in compatible extraction profile v5, and bind the batch to the exact Tool profiles before governance acceptance. |
| Review plaintext survives forever | A provider-free 128-attempt job expires/wipes shadow and pending proposals at day 30. |
| Compromised worker reads arbitrary tables | Runtime role has function execution only; owner membership and schema creation are forbidden. |
| Queue loss during Redis/worker outage | PostgreSQL rows remain pending and reclaimable. |

## Verification

- focused race tests for worker, chat capture, Redis wake, provider factory,
  runtime config, migration, and command packages;
- full backend tests and `go vet ./...`;
- disposable PostgreSQL migration up/down/re-up with atomicity, duplicate,
  stale lease, crash reclaim, cross-user denial, final-attempt dead-letter,
  candidate promotion/replay, evidence/profile drift, tombstone/conflict/
  temporary/Sensitive denial, projection enqueue, and direct-table denial proofs;
- preflight/Compose rendering and backend-image build.

## Change history

- 2026-07-28: Memory v2 PR3 durable outbox/jobs and private Go worker.
- 2026-07-28: Memory v2 PR4 evidence/revision-aware apply, epoch/tombstone
  no-resurrection fences, and provider-free online purge.
- 2026-07-28: Memory v2 PR5 strict candidate/decision proposals, Review shadow,
  canonical auto-apply revocation, and provider-free 30-day expiry.
- 2026-07-28: Memory v2 PR6 reuses the worker's strict/privacy contracts and
  records dead-letter outcomes as link-only assistant Activities.
- 2026-07-28: Memory v2 PR11 adds default-off, same-scope L2 Scene synthesis,
  derived embedding, and provider-free 24-hour stale purge lanes.
- 2026-07-29: Memory v2 PR12 adds default-off, stable Global-L1 L3 Persona
  synthesis, derived embedding, and provider-free 24-hour stale purge lanes.
- 2026-08-04: Migration `066` replaces L1 free-text extraction/decision with
  required Tool Calls and adds governance-backed, lease-fenced safe-add
  auto-capture promotion without changing reader authority.
- 2026-08-04: Migration `067` preserves the applied `066` bytes and hardens
  promotion with exact Tool-profile, batch-completeness, candidate-hash, and
  all-evidence currentness fences.
- 2026-08-04: Migration `068` preserves the applied `067` bytes and binds
  promotion to extraction profile v4, whose Tool schema enumerates hydrated
  user/assistant evidence IDs by role.
- 2026-08-04: Migration `069` preserves the applied `068` bytes and advances
  to compatible profile v5 by removing unsupported `uniqueItems`; bounded enums
  plus local duplicate/forgery rejection remain authoritative.

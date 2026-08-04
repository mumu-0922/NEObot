# Memory v2 candidate and Review shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`056_memory_candidate_review_shadow`, Memory extraction/decision prompts,
candidate batch persistence, Review suggestion routing, canonical temporal or
conflict metadata, or the worker's `review_expire` stage.

PR5 is proposal-only. It keeps the v1 Global reader and HTTP CRUD payloads,
adds no Review API/UI, and gives no automatic candidate authority to create,
merge, supersede, or otherwise mutate canonical `user_memories`.

The production L1 successor preserves that PR5 history but adds a deliberately
narrow safe-add authority in migrations `066`–`069`. It does not activate a
reader, change prompt authority, or authorize merge/supersede.

## 2. Signatures

The PR5 worker capabilities are:

```text
memory_worker_hydrate_capture_v2(UUID, UUID, UUID)
  RETURNS context messages, bounded current Memory context, Sensitive policy,
          current Project, Provider profile, and proposal_committed

memory_worker_propose_capture_candidates(
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
) RETURNS (proposal_count, shadow_count, review_count, rejected_count)

memory_worker_expire_capture_reviews(UUID, UUID, UUID) RETURNS INTEGER
```

The JSONB argument is an array of at most five objects with exactly these
keys:

```text
id, type, content, normalizedContent, candidateHash, importance, tags,
subjectKey, factKey, sensitivity, confidence, confidenceBand,
authorityUserMessageIds, contextMessageIds, confirmationKind,
proposedScopeType, proposedProjectId, proposedConversationId,
scopeConfidence, temporalBasis, temporalParserVersion, observedAt,
validFrom, validTo, factExpiresAt, proposedAction, targetMemoryIds
```

`memory_worker_runtime` loses execute permission on the PR4
`memory_worker_apply_capture_candidate(...)` capability after `056` is up.

The production successor adds:

```text
chat.ToolRoundProvider.CompleteToolRound(...)
  required call: propose_memory_candidates
  conditional required call: propose_memory_candidate_decisions

memory_worker_promote_capture_candidates(UUID, UUID, UUID)
  RETURNS (promoted_count, review_count, rejected_count)
```

## 3. Contracts

- `user_memories` gains additive lifecycle, subject/fact key, confidence,
  observed/valid/expiry/supersede, sensitivity, and temporal-parser fields.
  Existing rows backfill to active/normal/none; v1 readers and DTOs do not
  change.
- Provider output is strict JSON: every declared candidate/decision key must
  be present (nullable keys remain explicit); duplicate/unknown/trailing
  values, output over 32 KiB, more than five candidates, invalid enums/ranges/
  times, forged message IDs, and target IDs outside the hydrated bounded
  context are rejected before PostgreSQL proposal. The SQL capability repeats
  the exact-key check so a compromised worker cannot weaken this boundary.
- Context is at most eight completed user/assistant messages and 12,000
  redacted characters. Concrete secrets are removed before Provider egress.
  When Sensitive Memory is disabled, locally classified sensitive segments
  are also removed.
- Every candidate cites the current source user message. Assistant IDs are
  context-only; `confirmed_assistant` additionally requires at least one
  surviving assistant context ID.
- Go binds Project/Conversation IDs from the leased source. A model chooses
  only the semantic scope kind. A Project proposal without a current Project
  is narrowed to Conversation with zero scope confidence and therefore Review.
- One extract job commits one hash-pinned candidate batch atomically. A crash
  after proposal commit is resumed through `proposal_committed=true` without
  a second Provider call. A different replay payload is a conflict.
- Routing is deterministic before any future promotion: same-scope exact is
  shadow `NOOP` and its exact current revision is retained even when the
  related fact set overflows the five-target cap; a high-confidence unrelated `ADD` is shadow-only; low
  confidence/scope, relative or inferred time, manual conflicts, and
  `MERGE`/`SUPERSEDE` become pending Review. Cross-scope facts are overrides,
  not mutation targets.
- A local or model `secret` proposal stores only the candidate hash and result;
  content, normalized content, tags, subject key, and fact key are never
  persisted. Sensitive proposals are likewise rejected and wiped when the
  user's Sensitive switch is off.
- Shadow and pending proposal plaintext expires after 30 days. The
  provider-free `review_expire` job runs before hydration, changes them to
  `expired`, and clears all proposal plaintext while retaining ID/hash/reason/
  time/result authority. It has 128 attempts so bounded retry covers the
  24-hour purge SLA.
- Review target and evidence tables use composite ownership FKs and current
  canonical revisions. UUID arrays never substitute for database ownership.

### Production L1 successor authority

- Extraction and conditional decision are required Tool Calls, not free-text
  JSON. Each round accepts exactly one completed call with the exact name, a
  non-empty Provider-issued/non-synthetic call ID, no failure category, no
  prose fallback, and strict exact-key arguments. Missing, duplicate, wrong-
  name, failed, oversized, malformed, or unsupported Tool Round responses fail
  as sanitized `EXTRACTION_INVALID` after at most three total protocol attempts.
- Extraction profile v5 enumerates only the hydrated user-role IDs for
  `authorityUserMessageIds` and assistant-role IDs for `contextMessageIds` in
  each request's Tool schema. Provider output remains semantically revalidated.
- The batch keeps the existing exact candidate/decision JSON object contract.
  Tool transport changes framing, not the SQL payload schema.
- Migration `066` establishes lease-fenced safe-add promotion through
  `memory_governance_decide_review(...)`. Its applied bytes/checksum are
  immutable. Migration `067` is the forward authority fix for required Tool
  profile hashes, complete batch count/profile agreement, candidate hash, and
  currentness of every evidence message.
- Migration `068` preserves applied `067` bytes and binds promotion to the v4
  evidence-enumerated extraction profile; v3 batches are no longer authority.
- Migration `069` preserves applied `068` bytes and removes the unsupported
  `uniqueItems` keyword while keeping bounded role enums plus local duplicate/
  forgery rejection; promotion authorizes only profile v5.
- Promotion requires a current `shadow`/`SHADOW_ADD`/`ADD`, normal sensitivity,
  `explicit_user` or `confirmed_assistant`, no expiry/temporary inference,
  enabled Memory plus automatic recording, and current lease/outbox/source/
  assistant/Provider/scope/Project/epoch/evidence authority.
- Tombstone, existing exact/fact conflict, any related target, candidate drift,
  or stale evidence never creates canonical Memory. Eligible acceptance reuses
  the governance transaction and records `auto_accept`/`AUTO_CAPTURED` without
  exposing table CRUD to the worker.
- Crash replay returns the committed summary and creates no duplicate canonical
  row. Down migrations never delete canonical Memory or audit history.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Candidate batch is not an array, exceeds 64 KiB/5 items, or has unknown keys | `MEMORY_CANDIDATE_BATCH_INVALID` / `MEMORY_CANDIDATE_INVALID`; no partial batch. |
| Authority/context message is missing, cross-user, wrong-role, or deleted | `MEMORY_CANDIDATE_EVIDENCE_INVALID`; no proposal rows. |
| Proposed Project/Conversation is not the leased source scope | `MEMORY_CANDIDATE_SCOPE_INVALID`. |
| Decision target is forged, stale, cross-user, deleted, or outside proposed scope | `MEMORY_CANDIDATE_TARGET_INVALID`. |
| Secret object carries any plaintext/tag/key field | `MEMORY_SECRET_PLAINTEXT_FORBIDDEN`. |
| Candidate matches a tombstone | Hash-only rejected `TOMBSTONED`; never canonical. |
| Exact active row exists in the same scope | Shadow `EXACT_NOOP`; target revision recorded. |
| Related manual row differs | Pending `MANUAL_CONFLICT`; manual row unchanged. |
| Relative/inferred time or confidence/scope confidence below `0.80` | Pending Review; no canonical mutation. |
| Proposal transaction replays exact JSON/profile | Return the first batch counts idempotently. |
| Proposal transaction replays different JSON/profile | `MEMORY_CAPTURE_PROPOSAL_CONFLICT`. |
| Review expiry runs before its batch due time or against another user/event | `MEMORY_REVIEW_EXPIRY_TARGET_DRIFT`; retry without Provider. |
| Down sees proposal/Review/expiry history or non-default canonical metadata | `MEMORY_REVIEW_ROLLBACK_*`; preserve state. |
| Tool Round missing/duplicate/wrong-name/failed/oversized/malformed/synthetic | `EXTRACTION_INVALID` within three total protocol attempts; no prose fallback. |
| Batch Tool profile, count, or suggestion profile differs | `MEMORY_PROFILE_DRIFT`; rollback the promotion call. |
| Candidate content no longer hashes to `candidate_hash` | `AUTO_PROMOTION_CANDIDATE_DRIFT`; pending Review, no canonical write. |
| Any evidence role/content/completion/deletion/timestamp/Conversation is stale | `AUTO_PROMOTION_EVIDENCE_STALE` or primary source drift; no stale canonical write. |
| Tombstone, related target, exact row, or same-scope fact exists | `AUTO_PROMOTION_TOMBSTONED` / `AUTO_PROMOTION_CONFLICT`; pending Review, no resurrection/overwrite. |
| `067` down sees automatic promotion history | `MEMORY_AUTO_CAPTURE_AUTHORITY_ROLLBACK_REQUIRES_NO_PROMOTIONS`. |
| `068` down sees automatic promotion history | `MEMORY_AUTO_CAPTURE_TOOL_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS`. |
| `069` down sees automatic promotion history | `MEMORY_AUTO_CAPTURE_COMPATIBLE_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS`. |

## 5. Good / Base / Bad Cases

- **Good**: extraction returns five candidates; one high-confidence ADD remains
  shadow, an exact candidate becomes NOOP, a manual correction and relative
  time become pending Review, and a secret is hash-only rejected. Canonical
  Memory count and content remain byte-identical.
- **Base**: extraction returns `{"memories":[]}`. One zero-count batch pins the
  completed attempt; no expiry job or canonical row is created.
- **Bad**: let an N-1 worker call the old apply function, store candidate text
  in logs, trust model-supplied Project/target IDs, apply candidates one by one,
  resolve relative time using replay day, or hydrate a Provider for expiry.
- **Production successor Good**: one exact Tool-framed batch contains a current
  normal confirmed school fact; `069` retains the full `067` authority and governance
  atomically writes one canonical row, evidence, audit, and Activity.
- **Production successor Base**: empty batch completes without writes; temporary,
  conflicting, tombstoned, or Sensitive-disabled candidates remain Review or
  rejection exactly as routed.
- **Production successor Bad**: mutate deployed `066`–`069`, accept a synthesized Tool
  Call ID, promote a partial/profile-drifted batch, or duplicate governance SQL
  in Go.

## 6. Tests Required

- Go tests: strict missing/duplicate/unknown/trailing JSON, bounded output,
  generic English/Chinese secret assignments and Sensitive pre-egress
  redaction, evidence/scope/time validation, decision
  target spoofing, profile derivation, committed-proposal resume, and
  provider-free purge/review-expire dispatch.
- Static migration tests: all canonical fields/constraints, normalized Review
  target/evidence FKs, 30-day/plaintext shapes, old-apply revocation, 128-attempt
  expiry, narrow grants, and both down guards.
- Disposable PostgreSQL 17: replay `055 -> 056`, safe backfill, invalid scope/
  target atomic rollback, ADD/NOOP/manual-conflict/temporal/secret routing,
  canonical no-change, exact batch replay, direct-table denial, committed
  hydration, expiry plaintext wipe, guarded down, clean down, and re-up.
- Run focused race tests, all backend tests, `go vet ./...`, preflight/Compose,
  backend image build, and the full standalone gate. No test calls a Live
  Provider.
- Pin the live-applied `066`–`069` checksums and use disposable PostgreSQL 17
  to replay each forward fix through `068 -> 069 -> 068 -> 069`; assert
  profile/count/candidate/evidence fences,
  automatic acceptance atomicity, crash replay idempotency, tombstone/conflict/
  temporary/Sensitive outcomes, function-only denial, projection enqueue, and
  both promotion rollback guards.

## 7. Wrong vs Correct

### Wrong

```text
extract candidate 1 -> auto-apply canonical
extract candidate 2 -> insert Review
worker crashes -> replay and overwrite candidate 1 again
```

This exposes partial model output as authority and makes replay depend on
Provider nondeterminism.

### Correct

```text
strict extraction + bounded decision proposal
  -> one lease/source/scope/revision-fenced JSONB transaction
  -> hash-pinned shadow/pending/rejected batch
  -> canonical unchanged
  -> crash resumes from proposal_committed
  -> provider-free expiry clears plaintext at day 30
```

Production L1 successor:

```text
exact Provider-issued Tool Call framing
  -> unchanged strict candidate object contract + complete atomic batch
  -> migration-069 compatible profile + authority recheck
  -> governance safe-add only, otherwise Review/reject/fail closed
```

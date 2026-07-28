# Memory v2 candidate and Review shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`056_memory_candidate_review_shadow`, Memory extraction/decision prompts,
candidate batch persistence, Review suggestion routing, canonical temporal or
conflict metadata, or the worker's `review_expire` stage.

PR5 is proposal-only. It keeps the v1 Global reader and HTTP CRUD payloads,
adds no Review API/UI, and gives no automatic candidate authority to create,
merge, supersede, or otherwise mutate canonical `user_memories`.

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

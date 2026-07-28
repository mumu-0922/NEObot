# Memory v2 lexical projection and shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`058_memory_lexical_projection_shadow`, L1 exact/CJK BM25 projection
maintenance, lexical shadow comparison, shadow diagnostics, or the
`MEMORY_LEXICAL_SHADOW_ENABLED` rollout switch.

PR7 keeps the v1 in-process lexical reader as the only prompt and Usage
authority. It adds no vector, embedding Provider, RRF, reranker, token-budget
reader, promotion API, governance UI, L2/L3, Export/Import, or Hindsight.

## 2. Signatures

The fixed PR7 profile is:

```text
memory_lexical_cjk_bm25_v1
projection_generation = user_memory_state.active_projection_generation
PostgreSQL 17 + pg_textsearch 1.3.1 + text_config=simple
```

The only runtime-callable SQL capability is:

```text
memory_compare_lexical_shadow(
  observation_id UUID,
  user_id UUID,
  conversation_id UUID,
  assistant_message_id UUID,
  query_hash TEXT,
  query_text TEXT,
  v1_results JSONB,
  lexical_limit INTEGER
) RETURNS (
  observation_id, profile_id, projection_generation, status, result_code,
  baseline_count, exact_count, bm25_count, lexical_count, overlap_count,
  duration_millis
)
```

`v1_results` is an ordered array of at most five exact objects:

```json
{"memoryId":"uuid","revision":1,"scopeType":"global"}
```

The Go service exposes an optional shadow result while preserving the existing
`SearchRelevant` contract:

```go
SearchRelevantWithShadow(
    context.Context, query, conversationID, assistantMessageID string, limit int,
) ([]Memory, LexicalShadowSummary, error)
```

## 3. Contracts

- `user_memory_search_projections` is rebuildable derived plaintext. Every row
  binds canonical Memory/user, current revision/hash, scope, sensitivity,
  visibility epoch, scope generation, projection generation, fixed profile,
  normalized exact terms, and CJK BM25 text.
- PostgreSQL reuses the reviewed Knowledge BM25 normalizer/build function so
  projection and query use the same Latin/punctuation/CJK bigram semantics.
  No jieba, PGroonga, Search service, vector, or Provider is introduced.
- Internal `SECURITY DEFINER` triggers refresh projection in the canonical
  transaction for relevant insert/update fields. Deleted, disabled, non-active,
  or plaintext-purged canonical rows physically lose projection plaintext.
- Migration backfill refreshes every eligible current canonical row and fails
  unless projection count and every user/revision/hash/epoch/scope/profile/
  generation binding match exactly.
- Shadow comparison is valid only for the authenticated user's current
  streaming assistant and its completed user parent in the supplied active
  Conversation. SQL verifies that the transient query text and SHA-256 equal
  the parent message; neither is copied into durable result links.
- Candidate authorization occurs inside the indexed query: same user, allowed
  Global/current Project/current Conversation scope, enabled active canonical,
  valid current time, Sensitive switch, live epoch/scope generation, and exact
  revision/hash/profile/generation binding.
- Exact Top 20 and BM25 Top 30 remain distinct lanes. Their deterministic union
  produces lexical Top 20 with exact membership first and BM25 order second.
  PR7 does not introduce RRF or use raw score outside the query transaction.
- Observation parent stores only query/baseline hashes, profile/generation,
  bounded status/code/counts/duration, and IDs. Normalized result rows store
  lane/ordinal plus same-user Memory ID/revision/scope. They store no query,
  Memory content, prompt, embedding, Provider data, or raw score.
- The same assistant/query/baseline replay is idempotent. Query or ordered v1
  baseline drift returns `MEMORY_LEXICAL_SHADOW_REPLAY_CONFLICT` without
  rewriting the first observation.
- Shadow is default-off. Enabled comparison is fail-open for chat and fail-
  closed for shadow: v1 items, prompt, Usage, and response success remain
  byte-equivalent if projection/search/observation fails.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| PostgreSQL major/preload/extension version differs | Migration rejects with a bounded prerequisite code. |
| Canonical row is inserted or materially changed | Projection refreshes in the same transaction. |
| Canonical is deleted/disabled/non-active/purged | Projection row is physically removed immediately. |
| Projection revision/hash/epoch/generation drifts | Candidate is excluded before ranking. |
| Query/assistant/source/conversation ownership mismatches | `MEMORY_LEXICAL_SHADOW_SOURCE_INVALID`; no observation. |
| Sensitive switch is off | Sensitive projection cannot enter exact/BM25/lexical results. |
| Project/Conversation is archived/deleted or generation drifts | Scoped projection cannot enter a lane. |
| BM25 search fails after authority validation | Hash/ID-only failed observation; v1 answer continues. |
| Exact replay uses the same query and ordered v1 baseline | Return original observation/counts unchanged. |
| Replay changes query or baseline order/identity/revision | `MEMORY_LEXICAL_SHADOW_REPLAY_CONFLICT`. |
| Shadow env is absent/false | Zero compare calls and no shadow metadata. |
| Runtime attempts projection/observation table CRUD | PostgreSQL permission denied. |
| Down sees an active PR7 reader pointer | `MEMORY_LEXICAL_ROLLBACK_REQUIRES_V1_READER`. |
| Down sees observation history | `MEMORY_LEXICAL_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS`. |

## 5. Good/Base/Bad Cases

- **Good**: v1 injects one Global preference while exact/CJK BM25 independently
  finds a more specific current Project fact. The answer and Usage retain the
  v1 row; the ID-only observation records lane ranks and overlap for later
  benchmark analysis.
- **Base**: the flag is false or the query has no lexical signal. Projection
  remains correct, no Provider runs, no prompt changes, and either no compare
  occurs or a bounded empty result is recorded.
- **Bad**: rank cross-user rows then filter in Go, persist query/raw score,
  trust stale projection content, let a trigger run with table-owner search
  path, silently rewrite replay evidence, or inject shadow results before PR8
  and a separate promotion decision.

## 6. Tests Required

- Static migration tests: PG17/pg_textsearch prerequisite, schema/FKs/checks,
  exact/BM25 indexes, internal triggers, no query/content/raw-score observation
  fields, one runtime capability, narrow grants, and both down guards.
- PostgreSQL 17: full replay/backfill, CJK/Latin/punctuation retrieval, exact and
  BM25 lane independence/order, canonical create/update/direct correction/undo/
  delete/purge projection maintenance, stale revision/hash/epoch/generation,
  cross-user/scope/Sensitive/time/lifecycle exclusion, replay/conflict, direct-
  table denial, guarded down, clean down, and re-up.
- Go: default-off zero calls, enabled comparison input binding, sanitized
  summary, failure leaving v1 items/prompt/Usage unchanged, and metadata with no
  query/content/raw score.
- Run focused race, all backend tests, `go vet ./...`, preflight/Compose,
  backend image build, and the full standalone gate. No PR7 test calls a Live
  Provider or touches Live user Memory.

## 7. Wrong vs Correct

### Wrong

```text
BM25 probe across every projection
  -> Go filters user/scope/deletion
  -> shadow result enters prompt
  -> query and raw score are written to diagnostics
```

### Correct

```text
current user message + streaming assistant
  -> SQL-authorized exact and BM25 probes under current fences
  -> normalized ID/revision/rank-only observation
  -> v1 Top 5 remains the only prompt and Usage authority
  -> PR8/promotion stays a separate decision
```

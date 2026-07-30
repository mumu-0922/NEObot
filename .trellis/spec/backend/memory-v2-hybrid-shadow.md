# Memory v2 hybrid vector shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`059_memory_hybrid_vector_shadow`, additive migration
`064_memory_hybrid_relevance_admission`, additive migration
`065_memory_hybrid_final_hydration`, Memory BGE-M3 projection/jobs, hybrid
prepare/admission/record/final-hydration capabilities, RRF/rerank/cloud-judge/
main-model Tool routing/relevance/token selection, hybrid diagnostics,
`MEMORY_HYBRID_SHADOW_ENABLED`, or `MEMORY_TOOL_LOOP_ENABLED` wiring.

The deployed default keeps the v1 in-process Top 5 as prompt and Usage
authority. The additive product Memory Tool path is separately default-off and
has no Usage mutation or promotion authority. This slice adds no reader
promotion API, governance frontend, L2/L3, Export/Import, or Hindsight
execution.

## 2. Signatures

The fixed retrieval tuple is:

```text
memory_hybrid_bge_m3_rrf60_v1
siliconflow_bge_m3_v1
Pro/BAAI/bge-m3
1024 dimensions
Pro/BAAI/bge-reranker-v2-m3
RRF k = 60
candidate/final limits = 20/5
target/hard token budgets = 600/900
hard cutoff = 2 seconds
```

Memory Worker capabilities:

```text
memory_worker_claim_embedding_job(UUID, UUID, INTEGER)
memory_worker_hydrate_embedding_job(UUID, UUID, UUID)
memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[])
memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
)
```

Go API capabilities:

```text
memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
)
memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN,
  INTEGER
)
memory_authorize_hybrid_rerank(
  UUID, UUID, UUID, TEXT, REAL[]
)
memory_hydrate_hybrid_final(UUID, UUID, UUID)
```

The Go reader seam is:

```go
SearchRelevantWithHybridShadow(
    context.Context, query, conversationID, assistantMessageID string,
    limit int,
) ([]Memory, HybridShadowSummary, error)

SearchRelevantAfterMemoryToolCall(
    context.Context, HybridMemoryToolSearchInput,
) HybridMemoryToolSearchResult
```

The relevance-policy identities are:

```text
memory_hybrid_relevance_calibration_v1  calibration-only -1.00 / 0.00
memory_hybrid_relevance_intent_calibration_v1 query-intent -1.00 / scalar -1.00 / 0.00
memory_hybrid_cloud_candidate_judge_calibration_v1 cloud judge / scalar -1.00 / 0.00
memory_hybrid_main_model_tool_route_calibration_v1 main-model Tool route / scalar -1.00 / 0.00
memory_hybrid_main_model_first_tool_round_calibration_v1 first ToolRound / scalar -1.00 / 0.00
memory_hybrid_relevance_intent_abstention_v1 frozen Development-selected intent margin
owner_authorized_normal_candidates_v1 exact schema-v4 Provider-egress policy
owner_authorized_absolute_cap_v1 exact schema-v5 paid-model cost policy
memory-cloud-candidate-judge-prompt-v1
c004e834f2db572fc8393f088f47750d420379664f972357f987a09d8647f9c8
temperature-0_max-output-128_no-thinking_v1
search_memory
memory-search-tool-v1
f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6
memory-search-tool-decoding-v1
temperature=0 / maximum output=128 / thinking disabled
chat-first-tool-round-memory-decision-v1
```

The decoding/temperature/output/thinking tuple applies only to immutable
schema-v6 preflight evidence. The schema-v7 first-ToolRound profile binds the
adapter version and omits those preflight-only fields.

The strict judge output is exactly:

```json
{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[0,2]}
```

`selectedOrdinals` contains at most five unique in-range request-local
ordinals. An empty array is the only `no_memory` representation.

The main-model Tool definition has no arguments beyond an explicit empty
object:

```json
{
  "type": "function",
  "function": {
    "name": "search_memory",
    "description": "Search the current user’s saved personal Memory only when a prior preference, fact, decision, instruction, warning, correction, or project context is genuinely needed to answer the current request. Do not call for general knowledge, standalone tasks, content already present in visible conversation, or unrelated requests. The server enforces ownership and scope.",
    "parameters": {"type": "object", "additionalProperties": false}
  }
}
```

No Tool Call means `no_memory`. Use Memory requires exactly one call with a
non-empty ID, exact name, and explicitly decoded `{}` arguments.

## 3. Contracts

### Projection and embedding

- `user_memory_search_projections` owns one rebuildable fixed-profile
  `vector(1024)` plus `pending|ready|failed` state and a partial HNSW cosine
  index. Equal dimensions never authorize another model/profile.
- Canonical create/content/revision/hash changes set the vector to pending and
  idempotently replace its derived job in the same transaction. Delete,
  disable, non-active lifecycle, archive, or purge physically removes the
  projection/job.
- Scope, epoch, or projection-generation rebind may reuse a ready vector only
  because content/profile are unchanged. A pending/processing job is rebound
  or removed so its old response cannot cross the new authority.
- Every job pins user, Memory ID, revision, hash, visibility epoch, exact scope
  identity/generation, projection generation, embedding tuple, Provider record,
  and Provider `updated_at`.
- Claim admits exactly one active, enabled, attested `RAG:SILICONFLOW` record
  for the user. Duplicate eligible records fail closed to no claim.
- Provider attestation fingerprints concatenate UTF-8 `BYTEA` values with
  `decode('00','hex')`; PostgreSQL `TEXT` cannot contain `chr(0)` and must not
  be used to emulate NUL-delimited fingerprint bytes.
- Hydrate returns one bounded current Memory body and one encrypted Provider
  reference through the live lease. Complete repeats every job/current
  canonical/scope/time/Sensitive/generation/Provider fence before writing a
  finite, non-zero 1024-dimensional vector.
- The raw canonical body/hash remains the SQL lease authority. Immediately
  before Provider egress, the Worker creates a deterministic secret-redacted
  transient body. Partial redaction embeds only the surviving text; full
  redaction makes zero Provider calls and terminally fails the matching
  projection with bounded `EMBEDDING_SECRET_REDACTED` evidence.
- The Worker has no projection/job/provider/canonical table CRUD. Logs contain
  only job ID, Memory ID, bounded code, and status.
- Reclaiming an expired final lease changes the job to `dead_letter` and its
  still-matching projection to `failed/LEASE_EXPIRED` in the same transaction.
  A terminal job must never leave an unreclaimable false `pending` projection.

### Hybrid retrieval and recording

- `MEMORY_HYBRID_SHADOW_ENABLED` is default-off and gates embedding claims plus
  legacy API hybrid comparisons. `MEMORY_TOOL_LOOP_ENABLED` is a separate
  default-off API flag for first-round product Tool exposure and execution.
  Both false means zero Memory embedding/rerank/judge/Tool-route Provider calls.
  Product activation requires ready fixed-profile projections; operators must
  keep the Worker hybrid flag aligned when new/changed Memory must be embedded.
  Projection correctness does not depend on API Tool exposure.
- `SearchRelevantWithHybridShadow` always runs the v1 reader first; hybrid
  failure never changes its items, prompt, Usage links, or chat success.
  `SearchRelevantAfterMemoryToolCall` is a separate post-call product seam: it
  never invokes v1 and never falls back to v1 or unscored RRF candidates.
- Prepare accepts only the authenticated user's current streaming assistant,
  its exact completed user parent, and the current active Conversation/Project.
  Query text is transient and must match its SHA-256 and source message.
- SQL source/hash and lexical checks continue to use that raw query. Query
  embedding and rerank use deterministic secret-redacted transient copies.
  A fully redacted query records `query_embedding_status=redacted`, skips both
  Provider stages, and returns no hybrid final under bounded
  `SECRET_REDACTED`; a fully redacted candidate document skips rerank and also
  returns no hybrid final.
- Exact Top 20, CJK BM25 Top 30, and BGE cosine Top 30 apply user/scope/
  Sensitive/time/epoch/generation/revision/hash/profile fences inside each
  candidate query. Query embedding failure removes only the vector lane.
- Fusion is deterministic `sum(1/(60+lane_rank))`, deduplicated by Memory ID,
  with exact membership/rank and UUID tie-breaks. Raw lane/RRF scores are never
  persisted.
- Before any RRF Memory plaintext reaches the fixed BGE reranker, additive
  migration `064` reauthorizes the exact pending observation, source message,
  user, scope, revision/hash, visibility epoch, and projection generation. It
  computes only the maximum finite BGE cosine similarity from the supplied
  request-local query vector and persists neither that vector nor the score.
- Missing/stale admission evidence, a non-finite/out-of-range signal, a signal
  below the active versioned Provider threshold, query-embedding failure, or an
  unavailable policy sends zero Memory documents to the candidate stages and
  yields no hybrid final. It never substitutes v1 or unscored RRF candidates.
- When the intent policy is enabled, the fixed BGE reranker first receives only
  the secret-redacted query and two fixed, versioned, SHA-256-bound bilingual
  non-user anchors. Its request-local positive-minus-negative margin must pass
  the frozen Development threshold before local admission and before any
  candidate Memory document egress. Missing, malformed, model/anchor drift,
  timeout, or low margin fails closed to no hybrid final. Intent scores, query,
  and anchor bodies are never persisted.
- Under the exact schema-v4 owner policy, an otherwise current-authorized
  normal candidate excluded only as `irrelevant` may reach the configured
  cloud Provider. Cross-user, out-of-scope, deleted, secret, superseded,
  Sensitive-disabled, and untrusted-source candidates remain forbidden.
  Provider-egress authorization never changes the independent false-injection
  gate.
- Schema-v5 may bind a paid judge through
  `owner_authorized_absolute_cap_v1`. Its relative cost ratio is informational;
  exact official rates, 300-request/input/output ceilings, and maximum Memory
  Provider cost remain preauthorized and enforced. This economics profile does
  not weaken relevance, safety, latency, cutoff, token, split, or privacy
  gates, and schema-v4 evidence retains its original relative-cost semantics.
- The cloud judge receives only the deterministic secret-redacted query and
  candidate bodies labelled with contiguous request-local ordinals. It never
  receives Memory IDs, revisions, scopes, authority fields, RRF/BGE scores, or
  database metadata. Query and candidate bodies are untrusted data and cannot
  supply instructions or output authority.
- The judge contract accepts one exact-key JSON value only. Duplicate/unknown
  keys, trailing values, prose, Markdown, oversized output, wrong schema,
  duplicate/out-of-range ordinals, more than five selections, and non-integer
  values fail closed. Model ID, prompt version/SHA-256, decoding profile,
  `temperature=0`, output limit `128`, and no-thinking mode are immutable
  profile authority.
- For cloud-judge calibration, BGE rerank and the judge run concurrently under
  the same request-local stage context. Both must finish successfully. Judge
  ordinals are intersected with valid BGE order, then the existing final
  score/token selector runs. Empty judge selection records
  `CANDIDATE_JUDGE_ABSTAINED`; either failure, late result, or provenance drift
  records `CANDIDATE_JUDGE_FAILED` and yields no hybrid final.
- Concurrent result collection must select on the shared context rather than
  blindly receive both stage channels. A Provider that ignores cancellation
  must not hold the reader past the cutoff; stage result channels remain
  buffered so late goroutines cannot block while publishing discarded output.
- The historical schema-v6 Development alternative uses the current explicitly selected
  GPT or DeepSeek model through `HybridMemoryToolRouter`. The router receives
  only the deterministic secret-redacted current query and the fixed
  `search_memory` definition. It receives no candidate body, Memory ID, scope,
  revision, authority field, or retrieval score.
- Tool routing starts concurrently with query embedding. After prepare and
  migration-064 reauthorization, candidate BGE rerank may also overlap the
  route decision under the owner-authorized Provider-egress policy. Candidate
  bodies remain request-local to the BGE boundary and never enter the route
  model. Both route and BGE work count toward the existing hard cutoff.
- A route result is valid only when its exact model ID, contract version, and
  contract SHA-256 match policy authority. No call is a successful
  `MEMORY_TOOL_ROUTE_ABSTAINED`. Exactly one call with a non-empty ID, exact
  name, and non-nil empty argument object releases the unchanged BGE-scored
  Top-5/token selection. Missing/null/malformed/non-empty arguments, unknown or
  duplicate calls, Provider failure, cancellation, cutoff, or provenance drift
  is `MEMORY_TOOL_ROUTE_FAILED` and yields no hybrid final.
- Tool routing decides only whether saved Memory is needed. It cannot rewrite
  the query, select Memory IDs, authorize ownership/scope/revision, or authorize
  prompt injection. An empty candidate set still waits for the route decision
  to record `EMPTY`, `ABSTAINED`, or `FAILED` truthfully without inventing a
  final row.
- The schema-v6 adapter invoked an independent non-streaming
  `PlanTools` preflight. It does not share the existing chat
  `ToolRoundProvider` first answer round. Live Development rejected this
  preflight shape: GPT completed `41/300` route decisions, while corrected
  DeepSeek Flash completed `77/300`; neither profile passed.
- The DeepSeek Pro run that sent `enable_thinking=false` to the official
  `api.deepseek.com` host is `protocol_mismatch_invalid_quality_evidence`.
  Official DeepSeek requires `thinking.type=disabled`; generic compatible
  gateways retain `enable_thinking=false`, and official OpenAI omits both.
- Do not compensate by increasing the hard cutoff, retrying the preflight, or
  weakening gates. Schema-v6/profile-v6/cost-basis-v4 remain immutable failed
  evidence and cannot authorize another request shape.
- Product chat now exposes the canonical `search_memory` definition inside the
  existing first `StreamToolRound` only when `MEMORY_TOOL_LOOP_ENABLED=true`,
  the selected Provider is Tool-round capable, current Conversation policy
  allows Memory use, Search is not model-built-in, and the current turn is not
  a direct `remember|correct|forget` action. The flag defaults false and no live
  schema-v7 promotion decision has enabled it; the first GPT Development run
  failed unchanged gates.
- `internal/chat` owns the canonical definition/hash/validation boundary.
  `internal/memoryroute` is only a schema-v7 Development compatibility adapter
  that emits one real first `ProviderRoundRequest`; it does not own product
  continuation or Tool execution.
- Before the call, the Provider sees the normal conversation request and exact
  Tool but no Memory body. No Memory call performs zero hybrid retrieval. One
  exact first-round `search_memory({})` call starts the fixed BGE/RRF/rerank/
  Top-5/token path; the v1 reader and `MarkUsed` are not called.
- Retrieval failure and empty results return bounded Tool Results and still
  allow ordinary same-model continuation. `search_memory` is removed from all
  later rounds. A continuation failure before answer content recovers from the
  original request without any Memory body; partial content is never replayed.
- Rerank results retain finite `[0,1]` scores only in request memory. Invalid,
  duplicate, missing, failed, redacted, or cutoff output yields no hybrid
  final. Valid rows below the active policy's final threshold are removed before the
  existing Top-5 and 600/900-token selector. No retry can exceed the two-second
  request-local boundary.
- Provider return is not completion authority. Query embedding, rerank, and
  Worker embedding must inspect their stage context after the Provider returns;
  a Provider that ignores cancellation and returns apparent success after the
  deadline is treated as cutoff/retry and its output is discarded.
- Final selection contains at most five rows and uses a conservative
  multilingual estimator. It records whether the 600 target was exceeded but
  never exceeds 900 estimated tokens.
- Record revalidates the assistant/source, reader generation, user, scope,
  Sensitive switch, validity/expiry, canonical revision/hash, and projection
  for every submitted ID after Provider work. Drift produces `RESULT_STALE`
  and no stale final row.
- After a successful non-empty Record, migration `065` hydrates only the exact
  recorded final lane for the same authenticated user and streaming assistant.
  `SearchRelevantAfterMemoryToolCall` must preserve the original query bytes
  for `PrepareHybridShadow` and its SHA-256; `TrimSpace` may test emptiness but
  must not replace the value, because `065` binds the observation hash back to
  the exact source-message content.
  It repeats source hash, Conversation/Project lifecycle, settings, visibility
  epoch, projection generation, revision/hash, scope generation, time, and
  Sensitive fences. Any stale member rejects the whole final set. Go then
  verifies ordinal/ID/revision/scope/type equality and applies deterministic
  secret redaction again; a fully removed body fails closed.
- Exact assistant/query/ordered-v1/result replay is immutable. Same payload
  returns the first evidence; changed payload or result fails with
  `MEMORY_HYBRID_SHADOW_REPLAY_CONFLICT`.
- Durable observations/results store only hashes, profile/generation, IDs,
  revisions, scopes, lane ordinals, bounded status/fallback/count/token/
  duration values. They store no query, Memory body, prompt, embedding,
  Provider secret/authority, or raw score.

### Privilege and rollback

- `go_api_runtime` receives only hybrid prepare/admission/record/final-hydrate
  EXECUTE.
  `memory_worker_runtime` receives only embedding lease EXECUTE. Neither gets
  direct projection/job/observation CRUD or the other role's functions.
- All functions are `SECURITY DEFINER`, owned by `memory_runtime_owner`, and pin
  the application schema followed by `pg_catalog, pg_temp`.
- Migrations `064` and `065` are additive read-only capabilities. `065` down
  removes only final hydration; the clean `064 -> 065 -> 064 -> 065` replay
  must pass. Migration `059`
  down still requires the v1/NULL reader and empty hybrid observation history;
  never delete observation evidence to force rollback.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| PostgreSQL/pgvector/PR7 prerequisite differs | Migration aborts before adding PR8 objects. |
| Hybrid Worker/shadow flag absent/false | No embedding claim or legacy shadow comparison Provider call. |
| Provider missing, disabled, unattested, duplicated, or changed | No claim or bounded fallback/retry; never expose the credential. |
| Provider fingerprint is built through PostgreSQL `TEXT` plus `chr(0)` | Forbidden; build the exact NUL-delimited `BYTEA` sequence instead. |
| Job lease/revision/hash/epoch/scope/generation/profile drifts | Old response cannot complete; retry/dead-letter under bounded code. |
| Final embedding lease expires | Atomically dead-letter the job and fail the matching pending projection with `LEASE_EXPIRED`. |
| Embedding is wrong length, zero, NaN, or infinite | Reject; never mark projection ready. |
| Embedding body is fully secret-redacted | Zero Provider calls; terminal `EMBEDDING_SECRET_REDACTED` projection failure. |
| Query or rerank document is fully secret-redacted | Zero corresponding Provider calls; record `SECRET_REDACTED` and no hybrid final. |
| Query embedding fails | Exact/BM25 diagnostics and normal v1 chat continue; no admission, rerank document egress, or hybrid final. |
| Admission capability/evidence is missing or stale | Fail closed to no hybrid final and zero rerank document egress. |
| Maximum local cosine is below the active policy threshold | Record `RELEVANCE_ABSTAINED`; no candidate-document egress or hybrid final. |
| Query-intent signal is unavailable, invalid, late, drifted, or below threshold | Record `MEMORY_INTENT_FAILED` or `MEMORY_INTENT_ABSTAINED`; zero Memory-document egress and no hybrid final. |
| Cloud policy/model/prompt/decoding profile is absent or drifted | Record `POLICY_UNAVAILABLE` or `CANDIDATE_JUDGE_FAILED`; no hybrid final and no change to v1 chat. |
| Judge output has malformed JSON, extra/duplicate keys, prose, invalid cardinality, or invalid ordinals | Reject the entire result as `CANDIDATE_JUDGE_FAILED`; never partially accept it. |
| Judge returns an empty ordinal array | Record `CANDIDATE_JUDGE_ABSTAINED`, zero final rows, and zero prompt Memory tokens. |
| BGE or judge finishes late or ignores cancellation | Context selection returns without waiting, discards both candidate-stage results, and yields `no_memory`; no serial retry. |
| Candidate has a forbidden egress reason under the owner policy | Evaluation fails the zero-tolerance Provider-egress gate; only `irrelevant` is newly authorized. |
| Main-model Tool route returns no call | Record `MEMORY_TOOL_ROUTE_ABSTAINED`; discard speculative BGE final rows and record zero final/tokens. |
| Tool route returns a missing ID, wrong name, duplicate call, or nil/non-empty arguments | Reject the whole decision as `MEMORY_TOOL_ROUTE_FAILED`; never reinterpret it as an exact call. |
| Tool route model/contract provenance drifts or the Provider fails/is late | Record `MEMORY_TOOL_ROUTE_FAILED`, empty final, and unchanged v1 chat. |
| Tool route succeeds but the current-authorized BGE candidate set is empty | Record `MEMORY_TOOL_ROUTE_EMPTY`; do not fabricate a Memory result. |
| Official DeepSeek is sent generic `enable_thinking=false` | Mark the run protocol-invalid; it cannot support a model-quality conclusion. |
| The route is implemented as a separate pre-answer `PlanTools` request | Development-only failed hypothesis; never promote this request shape. |
| Product Memory Tool flag is absent/false | Do not expose `search_memory`; preserve the normal v1 prompt/Usage path. |
| Product first round returns no Memory call | Make zero hybrid retrieval calls and release the buffered ordinary answer. |
| Product first round returns one exact call | Execute the hybrid reader, record, hydrate through `065`, and continue on the same Provider/model. |
| Product query has leading/trailing whitespace | Preserve its exact bytes for prepare/hash/source identity; trim only for the empty-input check. |
| Final hydration count/identity/current authority drifts | Reject the complete set as `authority_stale`; return no Memory body. |
| Final body is fully secret-redacted after hydration | Fail closed; do not place it in a Tool Result. |
| Product continuation fails before content | Recover from the original request without Memory body. |
| Rerank fails, is invalid, or its reserved deadline expires | Record the bounded failure/cutoff and no hybrid final. |
| Every valid rerank score is below the frozen final threshold | Record `RELEVANCE_FINAL_ABSTAINED`, zero final rows, and zero estimated prompt Memory tokens. |
| Provider returns success after its context deadline | Discard the late output; never complete/rerank from it. |
| Submitted rerank/final ID is not current authorized RRF authority | `RESULT_STALE`, no stale final row. |
| Final estimate would exceed 900 | Skip that candidate; never exceed the hard budget. |
| Replay changes query, baseline, profile, or result | Conflict; first evidence remains unchanged. |
| Runtime attempts direct table access or cross-role function use | PostgreSQL permission denied. |
| Down sees promoted reader or hybrid history | Guarded refusal; schema remains applied. |

## 5. Good / Base / Bad Cases

- **Good**: exact and BM25 find a Chinese preference while BGE finds a
  paraphrase; RRF produces deterministic Top 20, the strict judge selects a
  directly useful ordinal, BGE order controls ranking, and a content-free
  observation records a budgeted final list while v1 remains the prompt/Usage
  source.
- **Base**: the flag is false, the production policy is unavailable, or query
  embedding/admission is unavailable. Canonical projection/jobs stay correct,
  chat uses v1, and no rerank Memory document is sent.
- **Bad**: claim with an arbitrary RAG record, reuse an old vector response
  after epoch/scope drift, rank cross-user then filter in Go, persist query or
  raw scores, accept free-form judge prose/IDs, treat owner egress authorization
  as injection authority, accept missing Tool arguments as `{}`, send candidate
  bodies to the route model, or inject Hybrid final IDs before a separate
  promotion decision.

## 6. Tests Required

- Go: default-off zero calls, fixed vector shape, capture/job fence drift,
  Provider and hydration retry classification, query/document/body redaction
  and secret-only zero-egress, admission unavailable/stale/low-similarity
  abstention, fixed intent-anchor hash, query-only intent egress,
  invalid/late/low-margin intent abstention, deterministic rerank-score
  validation, strict exact-key/duplicate-key/ordinal judge decoding, prompt
  SHA-256/model/decoding provenance, query/candidate secret redaction,
  concurrent BGE/judge failure and cutoff, ordinal intersection, empty-judge
  abstention, a Provider that ignores context without extending the cutoff,
  exact Tool definition/version/SHA-256, product default-off/direct-action/
  model-built-in exclusions, first-round buffering, no-call/exact-empty-object
  decisions, nil/non-empty/non-exact-name/unknown/duplicate/later-round
  rejection, multi-tool coexistence, exact query-byte/hash preservation,
  query-only Development adapter input, concurrent route/
  embedding/BGE completion, route failure/cutoff/empty-candidate handling,
  same-model continuation and body-free recovery, policy-aware
  Provider-egress scoring,
  post-threshold
  abstention, reserved cutoff recording, 600/900 token selection, bounded
  metadata, and byte-equivalent v1 prompt/Usage behavior.
- Static migration: fixed BGE tuple/HNSW, full job/final-hydration authority
  shape, no durable
  private payload/raw scores, three independent lanes, RRF(60), current record
  reauthorization, exact grants, and both down guards.
- PostgreSQL 17: full replay/backfill, fake 1024d claim/hydrate/complete/retry,
  final-lease projection failure, scope/epoch/provider old-response fences,
  duplicate-Provider fail-closed, exact/BM25/vector independence, RRF
  determinism, source/scope/vector admission drift denial, read-only admission,
  result stale, valid final hydration, logical-delete/revision/settings/
  projection-generation drift denial, exact runtime grant, replay conflict,
  worker/PUBLIC denial, guarded down, clean `064 -> 065 -> 064 -> 065`, and
  re-up.
- Run focused race, all backend tests, `go vet ./...`, Compose/preflight,
  backend image build, and the full standalone gate. No test calls a Live
  Provider or touches Live user Memory.

## 7. Wrong vs Correct

### Wrong

```text
claim any configured embedding provider
  -> store vector without epoch/scope/provider fence
  -> accept Provider success returned after its deadline
  -> mix BM25/cosine raw scores linearly
  -> rerank stale content
  -> write query/scores to diagnostics
  -> inject Hybrid Top 5
```

```go
// Wrong: migration 065 will compare this changed hash with the raw source row.
query := strings.TrimSpace(input.Query)
executeHybridShadow(query)
```

### Correct

```text
default-off hybrid-worker/shadow flag + separate default-off product Tool flag
  -> exact attested BGE job lease + current authority completion
  -> post-return deadline rejection + terminal lease/projection closure
  -> independently authorized exact/BM25/vector lanes
  -> deterministic RRF(60)
  -> local maximum-cosine admission with no durable vector/score
  -> historical strict cloud judge || Development route evidence
  -> product first ToolRound sees normal request + search_memory, no Memory body
  -> exact call: fixed BGE path + request-local score/token selection
  -> Record final -> migration-065 current-authority final hydration
  -> same-model continuation without search_memory
  -> content-free observation
  -> unchanged v1 prompt and Usage
```

```go
// Correct: validate normalized emptiness, but retain source identity bytes.
query := input.Query
if strings.TrimSpace(query) == "" {
    return noMemory
}
executeHybridShadow(query)
```

The schema-v6 `PlanTools` preflight is retained only as failed Development
evidence. Schema-v7 measures the implemented first-ToolRound shape. Its first
live GPT and DeepSeek Flash Development profiles both failed unchanged quality,
slice, cutoff, and latency gates. Production exposure stays default-off and
cannot be promoted without a passing Development/Validation result.

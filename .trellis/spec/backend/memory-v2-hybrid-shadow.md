# Memory v2 hybrid vector shadow contracts

## 1. Scope / Trigger

Apply this contract when changing migration
`059_memory_hybrid_vector_shadow`, additive migration
`064_memory_hybrid_relevance_admission`, additive migration
`065_memory_hybrid_final_hydration`, Memory BGE-M3 projection/jobs, hybrid
prepare/admission/record/final-hydration capabilities, RRF/rerank/cloud-judge/
configured-model candidate judging/main-model Tool routing/relevance/token
selection, hybrid diagnostics,
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

NewHybridMemoryToolRouteError(category string) error
HybridMemoryToolRouteFailureCategory(error) string
HybridMemoryToolRouteFailureCategories() []string
```

The failure-category list is sorted before hashing and contains exactly the
fixed 23 taxonomy values. Unknown causes map to
`ROUTER_FAILURE_UNCLASSIFIED`; callers must not synthesize dynamic values.

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
chat-configured-candidate-judge-v1
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

The measurement-only Judge failure diagnostic identities are:

```text
capture mode = development_fixed_memory_judge_failure_diagnostic
reader       = neo-chat.native-memory-reader-capture.v11
profile      = neo-chat.memory-regression-profile-config.v13
report       = neo-chat.memory-regression-relevance-calibration.v13
admission    = development_fixed_memory_judge_failure_diagnostic_only
artifact     = fixed-memory-judge-failure-diagnostic-development.json
taxonomy     = memory-candidate-judge-failure-taxonomy-v1
SHA-256      = c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d
```

The taxonomy is the sorted JSON array of the canonical 15 Provider categories
plus nine Judge-local categories. Provider categories come only from
`internal/chat`; Judge JSON/schema/ordinal categories come from typed decoder
stages. Unknown errors map to `CANDIDATE_JUDGE_FAILURE_UNCLASSIFIED`; callers
must never classify by matching error text.

The transport-stable identities and Go seams are:

```text
capture mode = development_fixed_memory_judge_transport_stable
reader       = neo-chat.native-memory-reader-capture.v12
profile      = neo-chat.memory-regression-profile-config.v14
report       = neo-chat.memory-regression-relevance-calibration.v14
admission    = development_fixed_memory_judge_transport_stable_only
artifact     = fixed-memory-judge-transport-stable-development.json
cost basis   = neo-chat.memory-regression-cost-basis.v9
```

```go
TransportStableDevelopmentExecutionPolicy(providerMode string) (
    AccuracyFirstExecutionPolicy, error,
)
WrapTransportStableMemoryJudgeDevelopmentProviders(...) (..., error)
CaptureTransportStableMemoryJudgeDevelopment(...) (CapturedProfile, error)
BuildTransportStableMemoryJudgeDevelopmentReport(...) (
    TransportStableMemoryJudgeDevelopmentReport, []byte, error,
)
BuildTransportStableMemoryJudgeRunManifest(...) (
    RelevanceRunManifest, []byte, error,
)
ValidateTransportStableMemoryJudgeCostAuthority(
    CostBasis, ConfiguredCandidateJudgeProfileAuthority,
) error
```

The separately owner-promoted product identity is:

```text
policy       = memory_hybrid_fixed_cloud_candidate_judge_production_v1
mode         = fixed_cloud_candidate_judge_production
reader order = fixed BGE rerank -> fixed Luna judge -> ordinal intersection
provider     = SERVER_DEFAULT / OpenAI Compatible
base URL SHA = 3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671
model        = gpt-5.6-luna
rollback     = MEMORY_TOOL_LOOP_ENABLED=false
```

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
- Once a route stage starts, it is part of the current capture-case lifecycle.
  Prepare failure/replay, query-embedding failure, admission failure or
  abstention, redaction, and empty candidates may not finish the case while the
  route is still able to publish. The stage exposes one immutable, replayable
  completion result so candidate execution and final lifecycle closure can
  both observe it without consuming a one-shot channel twice.
- The capture decorator is the only route-result writer. It runs the delegated
  router behind a buffered one-result channel and selects that channel against
  the route context. A delegated Provider that ignores cancellation cannot
  hold the reader or write Recorder state after the decorator has returned.
  Every route input returns a generation-bound Recorder token; result/failure
  writes from an earlier sequential case fail with
  `RECORDER_STATE_CONFLICT` instead of attaching to the current case, even if
  the assistant identity is reused.
- A route result is valid only when its exact model ID, contract version, and
  contract SHA-256 match policy authority. No call is a successful
  `MEMORY_TOOL_ROUTE_ABSTAINED`. Exactly one call with a non-empty ID, exact
  name, and non-nil empty argument object releases the unchanged BGE-scored
  Top-5/token selection. Missing/null/malformed/non-empty arguments, unknown or
  duplicate calls, Provider failure, cancellation, cutoff, or provenance drift
  is `MEMORY_TOOL_ROUTE_FAILED` and yields no hybrid final.
- Development diagnostics must preserve the request-local cause as one bounded
  typed category before the public fallback is collapsed. Schema v8 first
  introduced the route taxonomy; schema v9 is its executable route-only
  successor and aggregates HTTP authentication/quota/rate/request/upstream
  failures, transport and SSE failures, context termination, invalid Tool/event
  shapes, provenance drift, and recorder conflicts. It retains no Provider
  error text, response body, query, Tool payload, Memory body, score, or case
  identity.
  Fail-closed admission/rerank incompleteness is counted separately and may
  never retain a non-empty Final/Injected/token surface.
- The first live schema-v9 artifact classified the current run as `31`
  context deadlines, `83` invalid Tool Calls, and `174` unclassified router
  failures, with `174` separate admission-unavailable retrieval aggregates.
  These totals are diagnostic evidence, not a case-level join: equal counts
  cannot establish intersection or a more specific upstream cause.
- Offline source tracing after that run found that the executed reader started
  the route before query embedding but did not await it on non-empty-candidate
  admission-unavailable paths. `Recorder.Finish` could therefore observe a
  recorded route input with neither result nor category and synthesize
  `ROUTER_FAILURE_UNCLASSIFIED`; a delayed route could then race a later case.
  Deterministic regressions now close that lifecycle and generation-fence
  Recorder writes. This explains a concrete producer of the aggregate but
  does not rewrite the immutable identity-free v9 artifact or prove a
  case-level intersection.
- Schema v9 closes the candidate-blind Tool-route line. A route decision made
  before candidate recall cannot discover implicit personalization, and more
  route taxonomy cannot give it candidate knowledge. Preserve every v6-v9
  artifact and runtime seam as historical default-off evidence; do not use
  those results to select the successor policy.
- The schema-v10 Development successor recalls and reauthorizes candidates
  first, then reuses the strict ordinal judge with the exact configured GPT or
  DeepSeek Provider/model through adapter
  `chat-configured-candidate-judge-v1`. The Provider receives only the
  secret-redacted query and contiguous request-local candidate bodies, never
  IDs, scores, scope, revision, or database authority. Empty or invalid output
  fails closed; selected ordinals must still intersect fixed BGE order and pass
  post-Provider reauthorization before any future prompt injection.
- Profile schema v10, report schema v10, reader v8, cost-basis v6, exact
  Provider ID/type/Base-URL hash/model, and an independent mode-`0600`
  credential separate this hypothesis from historical SiliconFlow judges and
  Tool routes. GPT and DeepSeek are independent Development hypotheses. The
  authorized GPT run completed no candidate-bearing judge decision and failed
  recall/latency; the independent DeepSeek run completed `157/195` decisions
  but reached only `0.558974/0.581818` Final Recall@5/current-fact and also
  failed latency. Both retained zero false injection and zero authority/privacy
  leaks, but neither selected a policy. Validation, production composition,
  flag activation, and promotion remain unavailable.
- Schema-v10 report aggregation may retain a candidate-bearing pre-judge
  retrieval failure only when `AdmissionReady`, `RerankReady`, and
  `CloudJudgeReady` are false, judge token authority is zero, and Provider-
  sent/Final/Injected/token surfaces prove exact `no_memory`. It counts as a
  normalized failed case and not as an actual judge request. Schema-v4/v5
  report semantics remain immutable and reject the same state.
- The schema-v11 successor keeps the candidate-first topology but replaces
  answer-model-specific judging with one globally fixed cloud Memory Judge:
  `SERVER_DEFAULT`, `openai_compatible`, Base URL SHA-256
  `3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671`,
  and model alias `gpt-5.6-luna`. The observable tuple, adapter v1, prompt/hash,
  temperature-zero/no-thinking/max-output-128 decoding, fixed BGE tuple, and
  owner egress/cost authority are immutable regardless of the answer model.
- Schema v11 uses policy
  `memory_hybrid_fixed_cloud_candidate_judge_development_v1`, reader v9,
  profile/report v11, criteria v2, and cost-basis v7. The complete flow has
  p95 `<=1500 ms`, p99 `<=2500 ms`, and a `3000 ms` hard cutoff. Every other
  v1 quality, safety, token, and cost gate remains unchanged. Timeout,
  Provider error, invalid JSON, protocol drift, or a late result yields an
  empty v2 final set; recalled, reranked, schema-v10, and otherwise unjudged
  candidates are never fallback authority.
- The retained schema-v11 report is immutable failed evidence. Its short
  application cutoffs yielded only `41` Luna attempts and `22` complete
  rerank-plus-judge decisions, with `154` admission-unavailable cases and `19`
  `HARD_CUTOFF` complete stages. Later execution or criteria versions must not
  rescore, rewrite, or relabel that bundle.
- Schema v12 uses policy
  `memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2`, reader
  v10, profile/report v12, criteria v3, and cost-basis v8. Its exact path is
  query embedding, local admission, BGE rerank, Luna judge, and Record in that
  order. One controller serializes projection embedding and every per-case
  BGE/Luna request globally; no two Provider requests may overlap.
- Schema v12 adds no stage/case deadline and constructs both BGE and Luna HTTP
  clients without elapsed-time timeouts. Manual caller cancellation remains
  authoritative. Latency is aggregate diagnostic output only; any historical
  hard-cutoff flag/code is rejected as schema drift. A missing/invalid Provider
  result still fails closed to no v2 Memory for that case.
- Each Provider request may retry once only after `408`, `429`, `5xx`, or a
  retryable transport/read interruption. Valid `Retry-After` is honored;
  otherwise the fixed delay is five seconds. Redirects, normal `4xx`, invalid
  JSON/schema/protocol output, stream parse failures, and structured remote
  errors are not retryable. Live cases wait one real second between cases;
  fake protocol records the same 299 logical waits under a zero-elapsed virtual
  clock.
- Aggregate schema-v12 evidence reconciles attempts, retries, per-stage
  request latency, cooldown counts, total and retry Judge input-token bounds,
  and `JudgeAttempts * 128` output-token authority. Cost-basis v8 authorizes a
  maximum of 600 Judge attempts and 76800 output tokens. These execution and
  cost rules are Development evidence only and never install a runtime policy.
- Schema v13 replays the unchanged schema-v12 execution, policy, criteria, BGE,
  Luna, prompt, decoder, cost, and cooldown authorities only to classify Judge
  failures. It records one bounded category for every failed Judge attempt,
  including a recovered retry, and one terminal category for every
  `CANDIDATE_JUDGE_FAILED` case. Provenance drift and Recorder conflict are
  capture-local terminal failures and therefore are not Judge attempts.
- The schema-v13 aggregate must satisfy all three equations:
  `sum(terminal categories) = CANDIDATE_JUDGE_FAILED`,
  `sum(attempt categories) = JudgeRetries + terminal attempt-category failures`,
  and `JudgeAttempts = logical Judge requests + JudgeRetries`. Empty maps are
  valid when no Judge failure occurred; zero-valued or unknown map entries are
  forbidden. No category may retain error text, response bytes, query/Memory
  content, case identity, or selected ordinals.
- A schema-v13 report is measurement-only even when all 300 cases and every
  reconciliation complete: `diagnosticComplete=true` while
  `promotionEligible=false`, `policySelected=false`, and `passed=false`.
  Schema-v12 JSON/configuration omits every v13 field and remains immutable.
  This lane cannot change the prompt, corpus, threshold, policy, or reader and
  cannot authorize Validation, a paid run, or production activation.
- The retained schema-v13 live diagnostic run
  `memory-regression-20260804t005257z-8f43c5e7` reconciled `105` empty-
  candidate, `194` Judge-completed, and one failed case. Its `197` Judge
  attempts included two retries; failed attempts were one
  `PROVIDER_STREAM_READ_FAILED` and two `PROVIDER_TRANSPORT_FAILED`, and the
  sole terminal category was `PROVIDER_TRANSPORT_FAILED`. The independent
  evaluation passed at Candidate Recall@20 `1.0`, Final Recall@5
  `0.9948717949`, current-fact accuracy `0.9939393939`, false injection `0`,
  and zero safety violations. This does not override the mandatory schema-v13
  top-level non-passing/non-selecting state.
- Schema v14 implements that separately versioned Judge transport repair
  offline as `development_fixed_memory_judge_transport_stable`. It preserves
  the exact transient categories, permits two Judge retries with declared
  five/ten-second fallback waits and valid `Retry-After` precedence, keeps
  global Provider concurrency one, and requires cost-basis v9 authority for at
  most `900` Judge attempts and `115200` output tokens. Retrieval Provider
  retry ceilings remain unchanged. Focused Go and fake lifecycle gates pass.
  The consumed schema-v14 live run completed `105` empty-candidate plus `195`
  Judge-completed cases with zero failed cases/retries and passed every quality
  and safety gate. Its passing Development report remains non-selecting and
  non-promotional. Do not mutate v12/v13, change SSE/HTTP2/connection reuse,
  alter prompt/BGE/corpus/criteria, or rerun automatically from this result.
- The retained schema-v12 live result completed all `195` candidate-bearing
  rerank-plus-judge decisions with zero failed cases, but the accuracy-first
  policy injected Memory into `29/135` negative cases. Its false-injection
  rate `0.096667` failed the unchanged `0.02` maximum even though Candidate
  Recall@20, Final Recall@5, and current-fact accuracy reached
  `1.0/0.974359/0.969697` and every authority/privacy leak count stayed zero.
  Preserve this as immutable failed criteria-v3 evidence; do not promote,
  retune in place, enter Validation, or automatically rerun it.
- All v2 flags remain default-off. Development artifacts cannot activate a
  reader automatically. The owner separately promoted only the passing
  schema-v14 selection semantics for product `search_memory`; the Tool flag is
  still explicit rollback authority, and Tool failure never falls back to v1.
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
  a direct `remember|correct|forget` action. The flag defaults false. The owner
  separately enabled the fixed schema-v14 production policy; schema-v7 remains
  immutable failed routing evidence and grants no authority.
- A bounded bilingual gate over only the current user message recognizes
  explicit saved-Memory reads and direct personal recall questions. Those
  turns order `search_memory` first, use the existing named `required` choice,
  and disable optional reasoning only for the first decision round. General
  questions about memory and ordinary tasks remain Auto; direct
  remember/correct/forget actions retain their write path. Bounded first-person
  preference questions such as `我喜欢喝什么？` and
  `what do I like to drink?` are personal recall. Second-/third-person forms,
  advice such as `我应该喝什么？`, and quoted writing tasks are not. This gate
  selects whether to execute the Tool only. It does not select candidates or
  weaken fixed BGE/Luna release authority.
- Auto capability discovery remains fail-closed and non-blocking. Its fixed
  fictional probe uses thinking-disabled, temperature-zero, output-128
  settings and contains no user/Memory data. Official `api.deepseek.com` Tool
  rounds and Tool continuations use `thinking.type=disabled` with no
  `reasoning_effort`; plain no-Tool chat retains selected reasoning. An unknown
  explicit-read turn receives no Tool Memory until a later request observes a
  supported cache result.
- Live official-DeepSeek replay proved that a supported model may synthesize a
  forbidden `query` member even for the canonical zero-argument Memory Tool.
  The Provider adapter canonicalizes only a bounded valid JSON object returned
  for a server-declared zero-argument function to `{}` before validation and
  continuation. It never adopts the generated query. Malformed/non-object/
  oversized arguments remain denied, generic/argument-bearing Tools remain
  unchanged, and the canonical Memory Tool hash remains fixed.
- `internal/chat` owns the canonical definition/hash/validation boundary.
  `internal/memoryroute` is only a schema-v7 Development compatibility adapter
  that emits one real first `ProviderRoundRequest`; it does not own product
  continuation or Tool execution.
- Before the call, the Provider sees the normal conversation request and exact
  Tool but no Memory body. No Memory call performs zero hybrid retrieval. One
  exact first-round `search_memory({})` call requires the production policy,
  starts the fixed BGE/RRF/admission path, runs BGE rerank before fixed Luna
  judging, intersects exact ordinals in BGE order, then applies the Top-5/token
  path. The v1 reader and `MarkUsed` are not called.
- Every product Judge attempt re-resolves current stored `SERVER_DEFAULT` /
  OpenAI Compatible / attested Base-URL hash / `gpt-5.6-luna` authority. Missing
  dependency/secret or endpoint/model/prompt/decoder/policy drift fails closed.
  Only typed transient Provider failures receive at most two retries, with
  valid `Retry-After` precedence over fixed five/ten-second waits.
- Retrieval failure and empty results return bounded Tool Results and still
  allow ordinary same-model continuation. `search_memory` is removed from all
  later rounds. A continuation failure before answer content recovers from the
  original request without any Memory body; partial content is never replayed.
  The runtime carries only the exact context-budgeted Tool-result rows to
  assistant finalization. A completed continuation atomically records those
  current L1 revisions as immutable Usage; no-call, empty, failed, cancelled,
  or original-request recovery paths record no Tool Memory Usage.
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
| Schema-v10 retrieval fails before configured-judge egress | Retain one aggregate failure only with false admission/rerank/judge readiness, zero judge token authority, and empty Provider-sent/Final/Injected/prompt-token surfaces; do not increment judge requests and do not weaken historical schema-v4/v5 admission. |
| BGE or judge finishes late or ignores cancellation | Context selection returns without waiting, discards both candidate-stage results, and yields `no_memory`; no serial retry. |
| Schema-v12 fixed BGE or Luna request remains in flight without manual cancellation | Wait for the result; no application elapsed deadline may synthesize a cutoff. Global concurrency remains one. |
| Schema-v12 request returns 408/429/5xx or a retryable transport/read interruption | Honor valid `Retry-After`, otherwise wait five seconds, retry once, and account for both attempts. |
| Schema-v12 request returns a normal 4xx, redirect, invalid JSON/schema/protocol result, stream parse failure, or structured remote error | Do not retry; record the bounded fail-closed result and release no v2 Memory. |
| Schema-v12 trace contains `HardCutoffApplied`/`HARD_CUTOFF`, or telemetry/cost/cooldown counts do not reconcile | Reject the report as execution-policy drift; do not reinterpret it as an ordinary failed case. |
| Judge error has a typed Provider or JSON/schema/ordinal cause | Preserve only its fixed schema-v13 category; never parse or retain the error string. |
| A schema-v13 Judge retry recovers | Count the failed first attempt and the retry, but emit no terminal failure for the successful logical request. |
| A schema-v13 Judge request exhausts its retry or fails deterministically | Count every failed attempt and exactly one terminal category; release no v2 Memory. |
| Provenance drift or Recorder conflict occurs after a successful Judge attempt | Count one capture-local terminal category and no failed Judge attempt. |
| Schema-v13 terminal/attempt maps contain an unknown/zero value or fail any reconciliation equation | Reject the report and publish no bundle; never synthesize a dynamic category. |
| Schema-v13 completes with passing quality metrics | Keep `passed=false`, `policySelected=false`, and `promotionEligible=false`; diagnostic completion is not selection authority. |
| Schema-v14 Judge request returns a retryable typed failure | Honor valid `Retry-After`; otherwise wait five seconds before retry one and ten seconds before retry two. Keep BGE at one retry and global Provider concurrency at one. |
| Schema-v14 has any terminal failed case or fails attempt/terminal/cost reconciliation | Keep `passed=false`, release no v2 Memory, and reject drifted reports before publication. |
| Schema-v14 completes with zero failed cases and passing evaluation | The report may pass, but keep `policySelected=false` and `promotionEligible=false`; stop for owner review and never enter live/Validation automatically. |
| Candidate has a forbidden egress reason under the owner policy | Evaluation fails the zero-tolerance Provider-egress gate; only `irrelevant` is newly authorized. |
| Main-model Tool route returns no call | Record `MEMORY_TOOL_ROUTE_ABSTAINED`; discard speculative BGE final rows and record zero final/tokens. |
| Tool route returns a missing ID, wrong name, duplicate call, or nil/non-empty arguments | Reject the whole decision as `MEMORY_TOOL_ROUTE_FAILED`; never reinterpret it as an exact call. |
| Tool route model/contract provenance drifts or the Provider fails/is late | Record `MEMORY_TOOL_ROUTE_FAILED`, empty final, and unchanged v1 chat. |
| Retrieval becomes unavailable after the Tool route has started | Await the replayable route stage up to the existing hard cutoff, preserve the retrieval fallback, and expose no Final/Injected/token surface. |
| Delegated route Provider ignores cancellation | The decorator returns the bounded context category without waiting; any later delegated result can only enter its buffered channel and cannot mutate Recorder state. |
| A route result/failure carries a previous capture generation | Reject it as a Recorder state conflict; do not mutate the current case even when the assistant identity matches. |
| Diagnostic route failure category is empty, unknown, duplicated for one request, or conflicts with a successful route | Reject the capture state/report; each failed route contributes exactly one bounded category. |
| Tool route succeeds but the current-authorized BGE candidate set is empty | Record `MEMORY_TOOL_ROUTE_EMPTY`; do not fabricate a Memory result. |
| Official DeepSeek is sent generic `enable_thinking=false` | Mark the run protocol-invalid; it cannot support a model-quality conclusion. |
| The route is implemented as a separate pre-answer `PlanTools` request | Development-only failed hypothesis; never promote this request shape. |
| Product Memory Tool flag is absent/false | Do not expose `search_memory`; continue without Memory and never invoke the retired reader. |
| Current user explicitly requests saved Memory and capability is supported | Order `search_memory` first, select named `required`, and keep the fixed BGE/Luna selector as final release authority. |
| Current user discusses memory generally or submits an ordinary task | Preserve Auto; do not force Memory retrieval. |
| A known saved Memory still has `embedding_status=pending` because the private Worker is stopped | Product acceptance is not ready. Classify a candidate-empty Tool result as `memory_service_unavailable` or `memory_indexing`, restore the correctly flagged Worker, and wait for current projection readiness rather than bypassing BGE/Luna or adding a v1 fallback. |
| Explicit read sees unknown capability | Start the fixed background probe, release no Tool Memory for that turn, and never set an implicit override. |
| Official DeepSeek returns a JSON object with forbidden fields for canonical `search_memory` | Drop every returned member at the zero-argument Provider adapter boundary, validate canonical `{}`, and retain the current server-owned request as the only retrieval query. |
| Product Tool policy is absent or is any Development/shadow identity | Return `policy_unavailable`; perform zero hybrid Provider work and release no Memory. |
| Current stored fixed Judge provider/type/Base-URL hash/model/secret drifts | Reject that Judge attempt as provenance drift; release no final Memory and never switch Provider/model. |
| Product Judge returns a typed transient Provider failure | Retry at most twice; honor valid `Retry-After`, otherwise wait five then ten seconds. Deterministic/protocol/provenance failures do not retry. |
| Product first round returns no Memory call | Make zero hybrid retrieval calls and release the buffered ordinary answer. |
| Product first round returns one exact call | Execute the hybrid reader, record, hydrate through `065`, and continue on the same Provider/model. |
| Product query has leading/trailing whitespace | Preserve its exact bytes for prepare/hash/source identity; trim only for the empty-input check. |
| Final hydration count/identity/current authority drifts | Reject the complete set as `authority_stale`; return no Memory body. |
| Final body is fully secret-redacted after hydration | Fail closed; do not place it in a Tool Result. |
| Product continuation fails before content | Recover from the original request without Memory body. |
| Product continuation completes after a non-empty Memory Tool result | Persist immutable Usage for exactly the ordered, context-budgeted Tool-result rows in the assistant-finalize transaction. |
| Product Memory Tool is absent/empty/failed, or continuation uses original-request recovery | Persist zero Tool Memory Usage links. |
| Product retrieval returns `NO_CANDIDATES` and user health is `ready` | Return a successful empty Tool result; this is the only healthy miss. |
| Product retrieval returns `NO_CANDIDATES` and health is `indexing` | Fail the Tool step as `memory_indexing`; continue on the same model without Memory. |
| Product retrieval returns `NO_CANDIDATES` and health is `degraded`, `disabled`, or unreadable | Fail as `memory_service_unavailable`, `memory_disabled`, or `memory_status_unavailable`; never call v1. |
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
  product chat continues without Memory and without v1 fallback, no rerank
  Memory document is released, and any already-started diagnostic route is
  closed before the capture case finishes.
- **Configured-judge base**: private recall finds candidates for an unrelated
  request, but the exact configured judge returns an empty ordinal array; the
  answer prompt, Usage, and durable chat state receive no candidate body.
- **Configured-judge failure base**: recall finds candidates but admission
  fails before judge egress; schema v10 records a normalized failure with zero
  request/final/token surfaces while historical judge reports reject it.
- **Fixed-judge base**: fixed Luna returns strict empty ordinals or fails its
  bounded request; the complete v2 final set is empty, normal product chat
  continues without Memory or v1 fallback, and no recalled or reranked
  candidate reaches the prompt.
- **Health base**: `NO_CANDIDATES` plus ready heartbeat/projections is a normal
  empty result. The same retrieval surface with pending projection, missing
  Worker, or unreadable health is an explicit bounded failure and never a v1
  fallback.
- **Production good**: recall/rerank contains both the school and unrelated
  name fixtures; fixed Luna selects only the school ordinal, final hydration
  returns it, and assistant completion atomically persists exactly that one
  Usage link while the name remains rerank-only.
- **Production bad**: use `NO_CANDIDATES` alone as proof of a valid miss without
  consulting migration-070 current-user health.
- **Accuracy-first base**: the serial BGE stage completes but Luna returns a
  strict empty ordinal set; Record persists an empty counterfactual final,
  latency remains diagnostic, the next case starts only after its cooldown,
  and v1 remains the sole prompt/Usage authority.
- **Judge-diagnostic base**: the same serial request fails once with a typed
  `PROVIDER_RATE_LIMITED`, retries, and succeeds. Schema v13 increments the
  attempt map once, emits no terminal category, and still reports
  `passed=false` because diagnostic completion is not a quality gate.
- **Judge-diagnostic bad**: a terminal `CANDIDATE_JUDGE_FAILED` has no category,
  a retry failure is omitted from the attempt map, or a private Provider string
  becomes a map key. The report and bundle must be rejected.
- **Transport-stable good**: the Judge returns two retryable typed failures and
  succeeds on attempt three. Schema v14 records two failed attempts/two
  retries, waits five then ten seconds when no explicit advice exists, emits no
  terminal category, and may pass only if the independent evaluation passes.
- **Transport-stable base**: Judge succeeds on the first attempt or supplies a
  valid `Retry-After`; BGE still has at most one retry, the upstream delay wins,
  and a passing Development report remains non-selecting/non-promotional.
- **Transport-stable bad**: any logical Judge request remains terminal, the
  attempt/terminal equations drift, BGE receives a second retry, or the v9
  authority does not cover `900` requests and `115200` output tokens. Fail or
  reject the report without automatic rerun.
- **Bad**: claim with an arbitrary RAG record, reuse an old vector response
  after epoch/scope drift, rank cross-user then filter in Go, persist query or
  raw scores, accept free-form judge prose/IDs, treat owner egress authorization
  as injection authority, accept missing Tool arguments as `{}`, send candidate
  bodies to the route model, finish a capture while its route can still write,
  attach a late route result to the next case, or inject Hybrid final IDs
  before a separate promotion decision. It is also invalid to treat private
  candidate recall as prompt injection or to reuse Tool-route evidence for the
  configured candidate-aware judge.

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
  model-built-in exclusions, bilingual explicit-read/direct-preference
  positives and general-memory/other-subject/advice/quoted-task negatives,
  Memory-first named-required ordering, bounded fixed
  capability probes, official DeepSeek Tool/continuation versus plain-chat
  thinking shapes, zero-argument JSON-object canonicalization with malformed/
  generic/argument-bearing negatives, first-round buffering, no-call/exact-empty-object
  decisions, exact projected Tool-result Usage, zero Usage on empty/failure/
  cancellation/original-request recovery, nil/non-empty/non-exact-name/unknown/duplicate/later-round
  rejection, multi-tool coexistence, exact query-byte/hash preservation,
  query-only Development adapter input, concurrent route/
  embedding/BGE completion, route failure/cutoff/empty-candidate handling,
  route completion when query embedding/admission is unavailable, delegated
  Router cancellation ignorance without reader delay, previous-generation
  late-write rejection,
  exact 23-category ordering/hash, unknown-cause fallback, one-category-per-
  failure recorder semantics, raw-error exclusion,
  same-model continuation and body-free recovery, policy-aware
  Provider-egress scoring,
  schema-v10 exact configured Provider/profile/cost binding, strict flattened
  aggregate report shape, pre-judge retrieval-failure aggregation plus
  historical-report rejection, fake judge wiring, independent credential
  cleanup, and separate two-file manifest,
  schema-v11 exact Luna authority, criteria-v2 and 3000-ms cutoff binding,
  cost-basis-v7 drift denial, no-fallback fail-closed behavior, and an explicit
  Development-to-Validation stop,
  schema-v12 serial stage order/global concurrency, no-deadline/no-timeout
  behavior, diagnostic-only latency, hard-cutoff rejection, bounded transient
  retry classification and wait behavior, virtual/wall-clock cooldown,
  attempt/latency/input/output-token reconciliation, cost-basis-v8 ceilings,
  historical profile omission, and mandatory manual review,
  schema-v13 exact 24-category ordering/hash, Provider single-source reuse,
  typed JSON/schema/ordinal/event/oversize/context/unknown classification,
  recovered retry and retry-exhaustion attempt counts, terminal provenance and
  Recorder-conflict handling, all three reconciliation equations, 300-case
  fake report/manifest deterministic replay, v12 field omission, aggregate
  privacy scans, shell admission/credential/artifact validation, and permanent
  non-promotional/non-passing status,
  schema-v14 Judge-only two-retry recovery/exhaustion, exact five/ten-second
  fallback policy, explicit `Retry-After` precedence, unchanged one-retry BGE
  ceiling, historical v12/v13 field omission, 900-request/115200-output-token
  cost authority, zero-terminal pass semantics, fake bundle replay, and no
  automatic live authority, separate production policy identity, rejection of
  every non-production Tool policy, exact fixed Provider/type/Base-URL hash/
  model/secret drift denial, and production Judge retry/non-retry behavior,
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

```go
// Wrong: retrieval returned early, so the in-flight route can outlive the
// capture and write through whichever Recorder case is current later.
startRoute(caseContext)
if admissionUnavailable {
    return recordNoMemory()
}
```

```go
// Wrong: private and unstable error text becomes durable taxonomy authority.
if strings.Contains(err.Error(), "rate limit") {
    counts[err.Error()]++
}
```

```text
Wrong: raise the shared Provider retry ceiling for BGE and Judge together,
reuse cost-basis v8, then treat passing Development metrics as live authority.
```

```text
Wrong: install a Development policy in product chat, resolve Luna once at
startup, or fall back to v1 after Judge/provenance failure.
```

### Correct

```text
default-off hybrid-worker/shadow flag + separate default-off product Tool flag
  -> exact attested BGE job lease + current authority completion
  -> post-return deadline rejection + terminal lease/projection closure
  -> independently authorized exact/BM25/vector lanes
  -> deterministic RRF(60)
  -> local maximum-cosine admission with no durable vector/score
  -> private candidate recall and current-authority filtering
  -> historical strict cloud judge || Development route evidence
  -> schema-v10 configured-model/schema-v11 fixed Luna concurrent judge
     || fixed BGE rerank
  -> schema-v12 only: fixed BGE rerank -> fixed Luna judge, globally serial
  -> schema-v13 only: same serial flow + typed attempt/terminal aggregates,
     always non-selecting and non-passing
  -> schema-v14 only: same typed serial flow + Judge-only second retry,
     zero terminal failures required to pass and always non-selecting
  -> judge/BGE intersection; empty or uncertain result means no v2 Memory
  -> product first ToolRound sees normal request + search_memory, no Memory body
  -> exact call under production policy: current fixed Judge tuple reauthorized
     per attempt -> fixed BGE rerank -> fixed Luna ordinal intersection
  -> typed transient Judge failures only: Retry-After or fixed 5s/10s waits
  -> Record final -> migration-065 current-authority final hydration
  -> same-model continuation without search_memory
  -> content-free observation + exact Tool-result Usage on completed answer
  -> empty/failure/recovery: no Memory and no v1 fallback
```

```go
// Correct: validate normalized emptiness, but retain source identity bytes.
query := input.Query
if strings.TrimSpace(query) == "" {
    return noMemory
}
executeHybridShadow(query)
```

```go
// Correct: one replayable stage closes on every exit, while Recorder writes
// remain bound to the generation that recorded the route input.
route := startRoute(caseContext)
defer awaitRoute(caseContext, route)
if admissionUnavailable {
    awaitRoute(caseContext, route)
    return recordNoMemory()
}
```

```go
// Correct: classify only typed causes and reject an unreconciled aggregate.
category := memoryjudge.FailureCategory(err)
attemptCounts[category]++
if sum(attemptCounts) != judgeRetries+terminalAttemptFailures {
    return ErrCaptureInvalid
}
```

The schema-v6 `PlanTools` preflight is retained only as failed Development
evidence. Schema-v7 measures the implemented first-ToolRound shape. Its first
live GPT and DeepSeek Flash Development profiles both failed unchanged quality,
slice, cutoff, and latency gates. The later owner decision does not reinterpret
those results: it promotes only the separately passing schema-v14 fixed
BGE/Luna selection semantics, behind the default-off product Tool flag.

Schema-v7 does not contain route subtypes and remains immutable. Two schema-v8
attempts produced no artifact; the second stopped at bounded `admission_state`.
Schema v9 is the measurement-only successor: its hash-bound route categories
must sum to all failed routes and its retrieval-incomplete aggregates must
reconcile independently. It cannot freeze a policy, authorize Validation,
enable `MEMORY_TOOL_LOOP_ENABLED`, or reinterpret any v7/v8 run.

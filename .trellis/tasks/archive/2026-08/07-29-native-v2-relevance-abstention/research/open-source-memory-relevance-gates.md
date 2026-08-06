# Open-source Memory relevance gates and Neo Chat repair direction

## Scope and evidence boundary

This research was performed on 2026-08-01 after the immutable v4 and v5
Development runs. It uses checked-in source, retained aggregate reports, and
public open-source repositories only. It makes no Provider request, consumes
no credential or quota, and does not change any production reader, prompt,
corpus, threshold, report, or promotion authority.

The immediate evidence is deliberately narrow:

- v4 and v5 used disjoint hard-negative families but each retained exactly one
  `unrelated_negative` false selection out of 30;
- v5 also retained 17 `CANDIDATE_JUDGE_FAILED` cases, 217 Judge attempts, and
  22 retries;
- the aggregate-only bundle contains neither case identity nor raw Provider
  output, so it cannot establish whether those 17 cases were HTTP, transport,
  stream, JSON, schema, ordinal, or provenance failures; and
- positive-quality differences between v4 and v5 are therefore not causal
  evidence for another corpus change.

The goal of this document is to decide what to measure before changing the
relevance policy, and to identify which open-source patterns are safe to reuse.

## Upstream source pins

All observations below are bound to exact commits rather than floating branch
documentation.

| Project | Inspected commit |
| --- | --- |
| LlamaIndex | [`c864fcfa2c1d1f987ccdbcdab7b18e395c01ba86`](https://github.com/run-llama/llama_index/tree/c864fcfa2c1d1f987ccdbcdab7b18e395c01ba86) |
| Sentence Transformers | [`5de24293729a8905c2b0baf7bda76a130b221e31`](https://github.com/huggingface/sentence-transformers/tree/5de24293729a8905c2b0baf7bda76a130b221e31) |
| Haystack | [`44246f850714529353513a2a41e691bb1cf78b7e`](https://github.com/deepset-ai/haystack/tree/44246f850714529353513a2a41e691bb1cf78b7e) |
| Haystack core integrations | [`357d9b5b5c246d0dd59bbbf5b0c64ece27290303`](https://github.com/deepset-ai/haystack-core-integrations/tree/357d9b5b5c246d0dd59bbbf5b0c64ece27290303) |
| Mem0 | [`38e47ac2619b625ead46733db081251087f0c64b`](https://github.com/mem0ai/mem0/tree/38e47ac2619b625ead46733db081251087f0c64b) |
| Graphiti | [`4f62cfe7a2d519e55bfdf2dc4a2fd06649dc00b3`](https://github.com/getzep/graphiti/tree/4f62cfe7a2d519e55bfdf2dc4a2fd06649dc00b3) |
| LangChain | [`4cc62317852bd9ba548b9cc8932ef638529d0e6f`](https://github.com/langchain-ai/langchain/tree/4cc62317852bd9ba548b9cc8932ef638529d0e6f) |
| Self-RAG | [`1fcdc420e48f50a7d7ab1ece5494221b93252e99`](https://github.com/AkariAsai/self-rag/tree/1fcdc420e48f50a7d7ab1ece5494221b93252e99) |
| CRAG | [`de7c2961ae624a1483a138c5798e1f6d0c4fb0e0`](https://github.com/HuskyInSalt/CRAG/tree/de7c2961ae624a1483a138c5798e1f6d0c4fb0e0) |

## What the open-source systems actually do

### LlamaIndex: retain scores and allow a relevance cutoff

`SimilarityPostprocessor` rejects a node when its score is absent or below
`similarity_cutoff`; it does not treat an arbitrary Top-K result as sufficient
admission authority ([source](https://github.com/run-llama/llama_index/blob/c864fcfa2c1d1f987ccdbcdab7b18e395c01ba86/llama-index-core/llama_index/core/postprocessor/node.py#L70-L100)).
`LLMRerank` processes bounded batches, parses document choices plus per-choice
relevance, and sorts those retained scores before applying `top_n`
([source](https://github.com/run-llama/llama_index/blob/c864fcfa2c1d1f987ccdbcdab7b18e395c01ba86/llama-index-core/llama_index/core/postprocessor/llm_rerank.py#L82-L121)).
Its default prompt explicitly says to omit irrelevant documents and asks for a
1-10 relevance score per chosen document
([source](https://github.com/run-llama/llama_index/blob/c864fcfa2c1d1f987ccdbcdab7b18e395c01ba86/llama-index-core/llama_index/core/prompts/default_prompts.py#L462-L484)).

Useful pattern: bounded listwise judging with retained per-document relevance.

Do not copy blindly: this `LLMRerank` path still calls ordinary `llm.predict`
and parses model text. It does not solve Neo Chat's possible JSON/schema failure
class by itself.

### Sentence Transformers: score each query-document pair, then calibrate

`CrossEncoder.rank` builds scores for each query-document pair, sorts them, and
returns IDs/scores with an optional `top_k`
([source](https://github.com/huggingface/sentence-transformers/blob/5de24293729a8905c2b0baf7bda76a130b221e31/sentence_transformers/cross_encoder/model.py#L721-L838)).
`CrossEncoderRerankingEvaluator` evaluates a real first-stage candidate list or
explicit positives plus negatives and reports MAP/MRR/NDCG before and after
reranking
([source](https://github.com/huggingface/sentence-transformers/blob/5de24293729a8905c2b0baf7bda76a130b221e31/sentence_transformers/cross_encoder/evaluation/reranking.py#L19-L52),
[batching source](https://github.com/huggingface/sentence-transformers/blob/5de24293729a8905c2b0baf7bda76a130b221e31/sentence_transformers/cross_encoder/evaluation/reranking.py#L152-L215)).

Useful pattern: pairwise scores are suitable for a separately calibrated
admission threshold and can be batched locally.

Do not copy blindly: `rank` alone always ranks; it does not abstain. The
evaluator also requires a positive list and skips a sample with no relevant
documents, so Neo Chat's all-negative `unrelated_negative` slice must remain a
separate first-class gate. A threshold would need fresh calibration; the old
infeasible scalar sweep is not authority to invent one.

### Haystack: cross-encoder plus `score_threshold` can return no documents

`SentenceTransformersSimilarityRanker` exposes both `top_k` and
`score_threshold` ([source](https://github.com/deepset-ai/haystack-core-integrations/blob/357d9b5b5c246d0dd59bbbf5b0c64ece27290303/integrations/sentence_transformers/src/haystack_integrations/components/rankers/sentence_transformers/sentence_transformers_similarity.py#L40-L55)).
After ranking it filters every document below the threshold before applying
Top-K, so the output can be empty
([source](https://github.com/deepset-ai/haystack-core-integrations/blob/357d9b5b5c246d0dd59bbbf5b0c64ece27290303/integrations/sentence_transformers/src/haystack_integrations/components/rankers/sentence_transformers/sentence_transformers_similarity.py#L211-L295)).
Haystack's pipeline documentation places the ranker after a retriever and
recommends bounding the candidate count for efficiency
([source](https://github.com/deepset-ai/haystack/blob/44246f850714529353513a2a41e691bb1cf78b7e/docs-website/docs/pipeline-components/rankers/sentencetransformerssimilarityranker.mdx#L26-L36)).

Useful pattern: `recall -> bounded rerank -> threshold -> possibly empty` is
the correct control shape for Memory injection.

Do not copy blindly: a model-default threshold is not portable across BGE
models, score scaling, languages, or the current corpus.

### Mem0: semantic gate before hybrid boosts, but reranker failure is open

Mem0 OSS search defaults to semantic threshold `0.1` and optionally enables a
reranker
([source](https://github.com/mem0ai/mem0/blob/38e47ac2619b625ead46733db081251087f0c64b/mem0/memory/main.py#L1350-L1391)).
Its scoring function applies the semantic threshold before BM25 and entity
boosts, preventing a weak semantic candidate from being rescued only by a
secondary boost
([source](https://github.com/mem0ai/mem0/blob/38e47ac2619b625ead46733db081251087f0c64b/mem0/utils/scoring.py#L60-L139)).

Useful pattern: the relevance admission gate belongs before later rank/boost
signals can promote a candidate.

Do not copy blindly: if reranking throws, Mem0 logs a warning and returns the
original results
([source](https://github.com/mem0ai/mem0/blob/38e47ac2619b625ead46733db081251087f0c64b/mem0/memory/main.py#L1464-L1477)).
That fail-open policy is incompatible with Neo Chat's requirement that an
unjudged Memory never enter the answer prompt.

### Graphiti: hybrid recall, cross-encoder classification, and a minimum score

Graphiti exposes `reranker_min_score` in its search configuration
([source](https://github.com/getzep/graphiti/blob/4f62cfe7a2d519e55bfdf2dc4a2fd06649dc00b3/graphiti_core/search/search_config.py#L112-L129)).
Its cross-encoder path filters ranked facts below that minimum
([source](https://github.com/getzep/graphiti/blob/4f62cfe7a2d519e55bfdf2dc4a2fd06649dc00b3/graphiti_core/search/search.py#L395-L410)).
The OpenAI reranker asks for a Boolean relevance decision per passage, uses
True/False log probabilities as scores, and sorts them
([source](https://github.com/getzep/graphiti/blob/4f62cfe7a2d519e55bfdf2dc4a2fd06649dc00b3/graphiti_core/cross_encoder/openai_reranker_client.py#L34-L123)).

Useful pattern: judge every candidate relation rather than returning only an
unexplained selected subset, then apply an explicit minimum admission score.

Do not copy blindly: the OpenAI implementation issues one concurrent request
per passage. That multiplies requests and concurrency, conflicts with the
current Provider/WSL stability constraint, and depends on logprobs that the
configured OpenAI-compatible route has not proven.

### LangChain: structured output helps protocol integrity, not abstention by itself

`LLMListwiseRerank` refuses models without `with_structured_output` and binds a
Pydantic document-ID result schema
([source](https://github.com/langchain-ai/langchain/blob/4cc62317852bd9ba548b9cc8932ef638529d0e6f/libs/langchain/langchain_classic/retrievers/document_compressors/listwise_rerank.py#L40-L50),
[schema source](https://github.com/langchain-ai/langchain/blob/4cc62317852bd9ba548b9cc8932ef638529d0e6f/libs/langchain/langchain_classic/retrievers/document_compressors/listwise_rerank.py#L99-L143)).
`LLMChainFilter` instead asks for a Boolean result per document and can return
an empty list
([source](https://github.com/langchain-ai/langchain/blob/4cc62317852bd9ba548b9cc8932ef638529d0e6f/libs/langchain/langchain_classic/retrievers/document_compressors/chain_filter.py#L35-L110)).
`EmbeddingsFilter` combines Top-K with an optional similarity threshold and
can likewise return no document
([source](https://github.com/langchain-ai/langchain/blob/4cc62317852bd9ba548b9cc8932ef638529d0e6f/libs/langchain/langchain_classic/retrievers/document_compressors/embeddings_filter.py#L23-L97)).

Useful pattern: use Provider-native structured output for protocol authority,
and make filtering/abstention distinct from ranking.

Do not copy blindly: listwise ranking has no minimum relevance rule, while the
Boolean filter performs a batch of per-document LLM calls. Neo Chat should use
one bounded batch request with one verdict for every ordinal, not concurrent
per-candidate requests.

### Self-RAG and CRAG: trained critics, not prompt-only drop-ins

Self-RAG learns retrieval, relevance, support, and utility reflection tokens;
it can explicitly emit `[No Retrieval]` and `[Relevant]`
([source](https://github.com/AkariAsai/self-rag/blob/1fcdc420e48f50a7d7ab1ece5494221b93252e99/README.md#L7-L10),
[examples](https://github.com/AkariAsai/self-rag/blob/1fcdc420e48f50a7d7ab1ece5494221b93252e99/README.md#L75-L96)).
The repository trains separate Critic and Generator models with the reflection
tokens ([source](https://github.com/AkariAsai/self-rag/blob/1fcdc420e48f50a7d7ab1ece5494221b93252e99/README.md#L177-L193)).

CRAG uses a trained lightweight retrieval evaluator to emit a confidence that
drives correct, incorrect, or ambiguous actions
([source](https://github.com/HuskyInSalt/CRAG/blob/de7c2961ae624a1483a138c5798e1f6d0c4fb0e0/README.md#L6-L7),
[workflow](https://github.com/HuskyInSalt/CRAG/blob/de7c2961ae624a1483a138c5798e1f6d0c4fb0e0/README.md#L43-L57)).

Useful pattern: retrieval necessity, passage relevance, answer support, and
answer utility are different predicates and should not be collapsed.

Do not copy blindly: both systems rely on trained critic/evaluator behavior and
substantial inference infrastructure. Rewording the current Luna prompt does
not reproduce either architecture.

## Cross-project synthesis

The common safe shape is:

```text
high-recall retrieval
  -> current owner/scope/secret authorization
  -> bounded query-candidate scoring or verdicts
  -> calibrated relevance/necessity gate
  -> explicit empty-set outcome
  -> final token/Top-K selection
  -> answer prompt
```

No inspected project justifies any of these shortcuts:

- assuming Top-K is relevant merely because it was retrieved;
- inventing a score threshold without model/corpus calibration;
- treating structured output as a semantic relevance gate;
- allowing failed reranking/judging to fall back to unjudged candidates; or
- multiplying one Memory request into concurrent per-candidate LLM requests.

## Neo Chat's current information-loss point

The local source path is:

```text
chat.OpenAICompatibleProvider
  -> typed Provider failure category
memoryjudge.ChatAdapter
  -> stream collection / free-generated JSON decode
memorycapture.AccuracyFirstProviderController
  -> attempt, retry, latency and token counts only
memorycapture.CandidateJudgeDecorator
  -> records input and successful result only
usermemory hybrid final selection
  -> every Judge error becomes CANDIDATE_JUDGE_FAILED
cloud-judge Development aggregation
  -> counts only CANDIDATE_JUDGE_FAILED
```

The Provider already emits bounded plaintext-free categories for request build,
transport, response, authentication, quota, timeout, rate limit, request
rejection, upstream, stream parse/read/incomplete/remote-error, deadline, and
cancellation in
`mm-chat/backend/internal/chat/provider_failure.go`. The OpenAI-compatible
adapter assigns those types at HTTP and SSE boundaries.

The loss occurs later:

1. `memoryjudge.ChatAdapter` preserves typed Provider causes through joined
   errors, but its oversized output, unexpected event, JSON, schema, and ordinal
   errors are not typed.
2. `AccuracyFirstProviderController` decides retries from the typed Provider
   error but records only numeric attempt/retry totals.
3. `CandidateJudgeDecorator` has no failure recorder. It records only egress,
   input bounds, and a successful validated result.
4. `hybrid_shadow.go` maps all of those causes to
   `CANDIDATE_JUDGE_FAILED`.
5. `aggregateCloudJudgeDevelopment` therefore cannot recover the subtype.

This proves why changing the relevance prompt now would be blind: the 17 v5
failures may be semantic-independent execution failures.

## Recommended first change: versioned aggregate diagnostic only

Do not change prompt v1, corpus v2-v5, BGE, thresholds, selection, production
flags, or fail-closed behavior. Add a separately versioned Development-only
diagnostic whose sole authority is to classify future Judge failures.

### Bounded taxonomy

Reuse the existing `chat.ProviderFailureCategory` values directly rather than
parsing error strings or inventing a second HTTP/SSE taxonomy. Add only the
Judge-local categories that do not exist at the Provider layer:

```text
CANDIDATE_JUDGE_INPUT_INVALID
CANDIDATE_JUDGE_OUTPUT_TOO_LARGE
CANDIDATE_JUDGE_EVENT_INVALID
CANDIDATE_JUDGE_OUTPUT_JSON_INVALID
CANDIDATE_JUDGE_OUTPUT_SCHEMA_INVALID
CANDIDATE_JUDGE_OUTPUT_ORDINAL_INVALID
CANDIDATE_JUDGE_PROVENANCE_DRIFT
CANDIDATE_JUDGE_RECORDER_STATE_CONFLICT
CANDIDATE_JUDGE_FAILURE_UNCLASSIFIED
```

The sorted combined list must have a version and SHA-256. Unknown errors map to
the fixed unclassified value. No category may contain status text, exception
text, URL, request/response body, query, Memory content, ordinal selection, or
case identity.

JSON classification must be structural, not error-message parsing:

- invalid/empty/oversized/trailing JSON -> `OUTPUT_JSON_INVALID`;
- duplicate/unknown/missing keys, type/cardinality/schema-version drift ->
  `OUTPUT_SCHEMA_INVALID`; and
- duplicate or out-of-range ordinals -> `OUTPUT_ORDINAL_INVALID`.

### Aggregate data flow

Record two content-free views:

1. `judgeAttemptFailureCategoryCounts`: every failed Provider/adapter attempt,
   including a first attempt that later succeeds after the one allowed retry;
2. `judgeTerminalFailureCategoryCounts`: exactly one category for every case
   that ends as `CANDIDATE_JUDGE_FAILED`.

The attempt map belongs in `AccuracyFirstProviderController`. The terminal map
is derived from a new one-value Recorder field populated by
`CandidateJudgeDecorator`, so capture/provenance failures are not confused with
Provider attempts. Retained output contains only the two aggregate maps.

The diagnostic report must enforce at least these invariants:

```text
sum(terminal category counts)
  == failureCodeCounts["CANDIDATE_JUDGE_FAILED"]

sum(attempt category counts)
  == judgeRetries
     + terminal Provider/adapter failure count

judgeAttempts
  == logicalJudgeRequests + judgeRetries
```

Capture-local terminal categories such as provenance or Recorder conflict are
excluded from the second equation because the underlying Provider/adapter
attempt may have succeeded. Every category map must be non-nil, contain only
known values, use positive counts, and reconcile before publication.

### Version and privacy boundary

The diagnostic must use new profile/report/completeness identities and bind the
taxonomy version/SHA into configuration and manifest bytes. It may reuse the
immutable v5 corpus bytes as input, but it may not mutate or relabel the v5
quality report. It has diagnostic authority only:

```text
policySelected       = false
promotionEligible    = false
Validation           = unavailable
production activation = unavailable
```

No raw failure, response, output, case ID, query, Memory body, score, or
selected ordinal may enter a retained artifact or log.

### Offline proof before any live run

Use fake Providers and deterministic unit tests to cover every category and
reconciliation rule:

- each existing HTTP/transport/SSE typed failure;
- recovered retry and exhausted retry;
- invalid JSON, duplicate/extra/missing keys, schema drift, invalid ordinals;
- oversized output and unexpected Provider events;
- context cancellation, model provenance drift, and Recorder conflict;
- unknown errors mapping only to the fixed unclassified value;
- zero plaintext/case identity fields in marshalled output;
- fake-protocol 300-case report plus manifest byte replay; and
- unchanged fail-closed Final/Injected/prompt-token surfaces.

This work requires no Docker full gate and no Provider. Focused Go tests should
run with `GOMAXPROCS=2` and `GOFLAGS='-p=1'`; any later Compose check keeps
`COMPOSE_PARALLEL_LIMIT=1`.

## Decision tree after diagnostic evidence

### A. Failures are primarily HTTP, transport, stream, or context

Do not change the semantic prompt. Stabilize or replace the exact Provider
route, preserve global concurrency one, and keep fail-closed behavior. A retry
increase is not automatic: the current one-retry ceiling already exposes
request amplification and must remain cost-authorized.

### B. Failures are primarily JSON/schema/ordinal drift

Create a new adapter/profile using Provider-native required Tool or JSON-schema
structured output. Keep one request for the complete candidate batch. Require
one exact schema, validate every ordinal, and fail closed. LangChain validates
the structured-output mechanism, but Neo Chat must add its own abstention and
ordinal completeness rules.

The current generic `ProviderRequest` has no response-format field, while the
existing `ToolRoundProvider` supports required Tool calls. Tool output is the
smaller reuse surface, but the fixed Luna route's exact Tool capability is
unproven; it must first pass fake-protocol contract tests and later an explicitly
authorized capability run. Do not silently route through the historical
candidate-blind Memory Tool policy.

### C. Execution is stable but semantic false selection remains

Version a prompt/output contract rather than editing v1 in place. Combine the
open-source per-document verdict pattern with one bounded batch request:

```json
{
  "schemaVersion": "...v2",
  "verdicts": [
    {"ordinal": 0, "relation": "direct_useful"},
    {"ordinal": 1, "relation": "indirect_only"},
    {"ordinal": 2, "relation": "unrelated"}
  ]
}
```

Required semantics:

- exactly one verdict for every supplied ordinal;
- no missing, duplicate, or invented ordinal;
- only `direct_useful` may enter Final;
- `indirect_only` includes topical/background/entity-adjacent information that
  does not materially change the correct or personalized answer;
- `unrelated` has no necessary query-answer relation; and
- apply a counterfactual deletion test: if removing a Memory would not change
  the correct or personalized answer, it is not `direct_useful`.

This is intentionally stricter than the current selected-subset-only prompt and
separates ranking from admission. It still requires broad offline positive,
hard-negative, multilingual, injection, and mutation fixtures before any live
quality run.

### D. A local score gate is reconsidered

Reuse the existing BGE rerank score before adding another model or service.
Build new aggregate calibration curves on the exact successor corpus and keep
all-negative cases first-class. Do not reuse the old v2 scalar threshold or a
Mem0/Haystack default. Promote a threshold only if the unchanged recall,
current-fact, unrelated-negative, and safety gates all pass on the frozen
Development process.

## Explicitly rejected shortcuts

- No v6 corpus created to guess the identity-free v4/v5 false-positive case.
- No benchmark-specific subject/keyword blocklist.
- No relaxation of the `0.02` unrelated-negative gate.
- No Provider failure fallback to recalled or reranked candidates.
- No per-candidate concurrent LLM requests or two-Judge self-consistency run.
- No automatic retry increase, model hopping, Validation, production flag, or
  promotion action.
- No live diagnostic run without a fresh exact quota/cost authorization.

## Recommendation

Implement only the offline, versioned failure diagnostic first. It is the
smallest reversible change that converts the 17 collapsed v5 failures into an
actionable layer decision without changing relevance behavior. Present its
fake-protocol report and diff for owner review. Only after that review should a
separately authorized diagnostic run be considered; the resulting category
distribution decides whether the next repair belongs to Provider stability,
structured output, or semantic admission.

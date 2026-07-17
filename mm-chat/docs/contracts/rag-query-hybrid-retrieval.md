# RAG Query Embedding, Hybrid Retrieval, and Rerank Contract

## 1. Scope / Trigger

This contract applies when Go retrieves evidence from selected Knowledge
collections through the private Python/Jina query-embedding boundary, Postgres
hybrid candidate function, and private Python/Jina rerank boundary. It covers
G11.9C.2 and G11.9C.3, including global cross-collection final TopK.

## 2. Signatures

```http
POST /internal/retrieval/query-embedding
Authorization: Bearer <RAG_SOURCE_GATEWAY_TOKEN>
Content-Type: application/json

{"query":"..."}
```

```http
POST /internal/retrieval/rerank
Authorization: Bearer <RAG_SOURCE_GATEWAY_TOKEN>
Content-Type: application/json

{"query":"...","documents":["..."]}
```

```sql
knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
)
```

```go
type QueryEmbedder interface {
    EmbedQuery(context.Context, string) (QueryEmbedding, error)
}

type Reranker interface {
    Rerank(context.Context, string, []string) ([]RerankResult, error)
}
```

## 3. Contracts

- Request body: one exact `query` string; body at most 4096 bytes; trimmed query
  at most 2048 UTF-8 bytes.
- Response: exact `model`, `dimensions`, and `embedding` fields; model must be
  `jina-embeddings-v4`, dimensions and vector length must both be 1024.
- Jina task: query uses `retrieval.query`; indexed chunks remain
  `retrieval.passage`.
- Environment: `RAG_QUERY_GATEWAY_URL` locates the private Python service;
  `RAG_SOURCE_GATEWAY_TOKEN` authenticates the internal request. A blank token
  disables the query client and preserves keyword-only retrieval.
- `RAG_RERANK_GATEWAY_URL` locates the private rerank service and falls back to
  `RAG_QUERY_GATEWAY_URL` when omitted. Go disables the client when the shared
  internal token is blank.
- Postgres returns reference fields and rank only. Source body hydration remains
  a later Go reauthorization step.
- Dense candidates require at least eight trimmed query characters and cosine
  `>= 0.48`, then merge with keyword/CJK candidates through RRF `k=60`.
- Go globally fuses at most 20 references, authorizes rerank consent for the
  owner query and every selected collection, and hydrates/reauthorizes source
  text in bounded `16 + 4` batches before any source text leaves Go.
- Jina receives the rewritten standalone query when one exists, otherwise the
  original query, plus at most 20 authorized documents. The provider request
  pins `jina-reranker-v3`, `top_n=document count`,
  `return_documents=false`, and `return_embeddings=false`.
- A successful rerank must return every input index exactly once with a finite
  score. Go keeps scores `>= 0.0`, sorts globally across all selected
  collections, and injects at most five chunks.
- Rerank and Answer are query-time collection authorities. Granting, revoking,
  or expiring rerank-only/answer-only consent must not advance
  `collection_processing_revision` or invalidate an already published search
  materialization. Parse or passage-embedding consent changes still do.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Missing/invalid internal Bearer | fixed `401 UNAUTHORIZED` |
| Non-JSON or unknown/missing body field | fixed `4xx` error without query echo |
| Jina unavailable or provider response invalid | fixed private `503`; Go retries keyword retrieval |
| Vector not 1024, non-finite, or zero norm | reject before Postgres |
| Query gateway disabled | keyword-only retrieval |
| Hybrid Postgres query fails | dependency error; do not disguise as a normal miss |
| Candidate fails current projection/visibility/deletion fences | omit reference |
| Go hydration reauthorization fails | omit evidence / follow Auto degradation policy |
| Rerank governance or consent absent | do not send source text; retain bounded hybrid/RRF order |
| Consent/DB lookup fails | dependency error; do not disguise as a normal miss |
| Rerank provider unavailable or response invalid | retain hybrid/RRF order, Top5, `rerankStatus=degraded` |
| Rerank succeeds but every score is below `0.0` | normal `no_evidence` |
| Rerank succeeds with valid scores | global threshold/Top5, `rerankStatus=applied` |

No error may contain the query, internal token, Jina key, provider response, or
source text.

## 5. Good / Base / Bad Cases

- Good: a long Chinese semantic paraphrase with no lexical candidate crosses
  the Dense gate and returns the active selected-collection reference.
- Base: an exact phrase is present in both keyword and Dense lanes and receives
  deterministic fused rank.
- Bad: a short weather query cannot enter the Dense-only lane even when its raw
  embedding cosine is spuriously high; unrelated longer queries below `0.48`
  also return no Dense reference.
- Rerank good: two selected collections both produce authorized candidates;
  the provider returns finite scores and Go mints citations in one global order.
- Rerank base: the rerank endpoint alone is unavailable; the same keyword hit
  still answers with a citation from pre-rerank hybrid/RRF order.
- Rerank bad: governance is absent or an index is duplicated/missing/non-finite;
  no unauthorized text is sent and invalid provider output is never applied.

## 6. Tests Required

- Python unit: auth, exact body shape, byte bounds, Jina `retrieval.query`
  request, response/model/vector validation, and redacted failures.
- Go unit: URL/token validation, redirect rejection, bounded response decoding,
  hybrid selection, keyword fallback, and visible database failure.
- Postgres integration: migrations compile from an empty database; runtime roles
  can execute the function; no-lexical-overlap Dense positive returns a
  reference; short Dense-only negative returns none.
- Live smoke: real Jina returns one finite 1024 vector; semantic positive yields
  `answered/[K1]`; stopping Jina still lets a keyword positive yield
  `answered/[K1]`; all disposable rows and databases are removed.
- Rerank unit: exact private request/response shape, auth and byte limits,
  redirect rejection, hydration `16 + 4`, consent-before-egress, threshold,
  global Top5, invalid-response fallback, and query-time consent revision rules.
- Rerank live: real private endpoint returns the pinned model; normal chat yields
  `rerankStatus=applied`; rerank-only failure yields `degraded` while still
  answering with `[K1]`; two selected collections yield one applied global
  result with citations from both; disposable conversations are removed.

## 7. Wrong vs Correct

Wrong: treat raw cosine as a calibrated relevance probability, send candidate
text before exact consent/hydration, invalidate the index for query-time
consent, or silently fall back after a Postgres failure.

Correct: use the conservative pre-rerank signal gates, keep Jina failure
keyword-degradable, keep database failure observable, and apply the evaluated
rerank threshold only after current-authority hydration.

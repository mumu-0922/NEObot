# RAG Query Embedding and Hybrid Retrieval Contract

## 1. Scope / Trigger

This contract applies when Go retrieves evidence from selected Knowledge
collections through the private Python/Jina query-embedding boundary and
Postgres hybrid candidate function. It covers G11.9C.2 only; Jina reranking and
cross-collection final TopK policy remain G11.9C.3.

## 2. Signatures

```http
POST /internal/retrieval/query-embedding
Authorization: Bearer <RAG_SOURCE_GATEWAY_TOKEN>
Content-Type: application/json

{"query":"..."}
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
- Postgres returns reference fields and rank only. Source body hydration remains
  a later Go reauthorization step.
- Dense candidates require at least eight trimmed query characters and cosine
  `>= 0.48`, then merge with keyword/CJK candidates through RRF `k=60`.

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

## 7. Wrong vs Correct

Wrong: treat raw cosine as a calibrated relevance probability or silently fall
back after a Postgres failure.

Correct: use the conservative pre-rerank signal gates, keep Jina failure
keyword-degradable, keep database failure observable, and defer production
relevance calibration to the explicit rerank slice.

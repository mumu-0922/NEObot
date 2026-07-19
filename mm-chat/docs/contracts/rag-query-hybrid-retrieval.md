# RAG Query Embedding, Hybrid Retrieval, and Rerank Contract

## 1. Scope / Trigger

This contract applies when Go retrieves evidence from selected Knowledge
collections through direct Jina query embedding, the Postgres hybrid candidate
function, and direct Jina reranking. It covers G11.9C.2, G11.9C.3, and the
G11.9F.4.4 provider-boundary cutover, including global cross-collection TopK.

Python is not on the query-time retrieval path. It uses the scoped Go provider
gateway only for background `retrieval.passage` batches and MinerU jobs.

## 2. Signatures

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

The Go `ProviderGateway` implements both interfaces. It resolves the enabled,
attested `RAG:JINA` Postgres/vault record for every call and synthesizes the
fixed Jina request. No browser or Python caller supplies an upstream URL,
header, model, task, or credential.

## 3. Contracts

- Query input is trimmed, non-empty, and at most 2048 UTF-8 bytes.
- Query embedding pins `jina-embeddings-v4`, `retrieval.query`, and 1024
  dimensions. The returned vector must be finite, non-zero, and exactly 1024
  elements.
- Indexed chunks remain `retrieval.passage`; Python sends them only to
  `POST /internal/rag/providers/jina/embeddings` with the infrastructure
  internal token. Go resolves and uses the reusable Jina credential.
- There is no query/rerank URL environment setting and no Go-to-Python
  provider hop. Missing, disabled, corrupt, or unattested `RAG:JINA` state is
  treated as provider unavailability and preserves the documented degraded
  retrieval behavior.
- Postgres returns reference fields and rank only. Source-body hydration
  remains a later Go reauthorization step.
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
- The provider client ignores environment proxies, follows no redirects,
  requires TLS 1.2 or newer, accepts identity-encoded JSON only, and bounds the
  response before decoding.
- Rerank and Answer are query-time collection authorities. Granting, revoking,
  or expiring rerank-only/answer-only consent must not advance
  `collection_processing_revision` or invalidate an already published search
  materialization. Parse or passage-embedding consent changes still do.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Query empty, oversized, or invalid | reject before provider access |
| `RAG:JINA` absent, disabled, unattested, or vault unavailable | provider unavailable; retain allowed degraded path |
| Jina unavailable or response invalid | keyword/hybrid fallback where policy permits |
| Vector not 1024, non-finite, or zero norm | reject before Postgres |
| Hybrid Postgres query fails | dependency error; do not disguise as a normal miss |
| Candidate fails current projection/visibility/deletion fences | omit reference |
| Go hydration reauthorization fails | omit evidence / follow Auto degradation policy |
| Rerank governance or consent absent | do not send source text; retain bounded hybrid/RRF order |
| Consent/DB lookup fails | dependency error; do not disguise as a normal miss |
| Rerank provider unavailable or response invalid | retain hybrid/RRF order, Top5, `rerankStatus=degraded` |
| Rerank succeeds but every score is below `0.0` | normal `no_evidence` |
| Rerank succeeds with valid scores | global threshold/Top5, `rerankStatus=applied` |

No error or log may contain the query, vault envelope, reusable Jina key,
provider response body, or source text.

## 5. Good / Base / Bad Cases

- Good: a long Chinese semantic paraphrase with no lexical candidate crosses
  the Dense gate and returns the active selected-collection reference.
- Base: an exact phrase is present in both keyword and Dense lanes and receives
  deterministic fused rank.
- Bad: a short weather query cannot enter the Dense-only lane even when its raw
  embedding cosine is spuriously high; unrelated longer queries below `0.48`
  also return no Dense reference.
- Rerank good: two selected collections both produce authorized candidates;
  Jina returns finite scores and Go mints citations in one global order.
- Rerank base: only rerank is unavailable; the same keyword hit still answers
  with a citation from pre-rerank hybrid/RRF order.
- Rerank bad: governance is absent or an index is duplicated, missing, or
  non-finite; no unauthorized text is sent and invalid provider output is never
  applied.

## 6. Tests Required

- Go unit: exact direct Jina URL/header/model/task shapes, credential resolution,
  redirect rejection, bounded response decoding, vector validation, hybrid
  selection, keyword fallback, rerank validation, and visible database failure.
- Python unit: the retired `/internal/retrieval/query-embedding` and
  `/internal/retrieval/rerank` paths return `404`; passage embedding uses only
  the scoped Go DTO and internal token.
- Postgres integration: migrations compile from an empty database; runtime
  roles can execute the function; no-lexical-overlap Dense positive returns a
  reference; short Dense-only negative returns none.
- Live smoke: the stored/activated Jina provider returns one finite 1024 vector;
  semantic positive yields `answered/[K1]`; provider degradation still lets a
  keyword positive yield `answered/[K1]`; disposable rows are removed.
- Rerank live: normal chat yields `rerankStatus=applied`; rerank-only failure
  yields `degraded` while still answering with `[K1]`; two selected collections
  yield one applied global result with citations from both.

## 7. Wrong vs Correct

Wrong: keep a Go -> Python -> Go provider cycle, read a Jina key from Python
environment variables, treat raw cosine as calibrated relevance, send candidate
text before exact consent/hydration, or silently mask a Postgres failure.

Correct: Go resolves the attested Postgres/vault credential and calls Jina
directly, uses conservative pre-rerank gates, keeps provider failure
keyword-degradable, keeps database failure observable, and applies the evaluated
rerank threshold only after current-authority hydration.

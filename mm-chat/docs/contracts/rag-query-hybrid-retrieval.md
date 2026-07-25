# RAG Query Embedding, Hybrid Retrieval, and Rerank Contract

## 1. Scope / Trigger

This contract applies when Go retrieves evidence from selected Knowledge
collections through a Generation-selected Search Profile. Migration `050`
permanently retires Jina execution. Historical Jina Generation/Profile/vector
rows remain schema-valid audit evidence, but they cannot authorize Query
Embedding, Dense search, Rerank, configuration, credential resolution, or
rollback.

Until the verified SiliconFlow Candidate receives explicit Activation
approval, the legacy Active Generation is served through its same-Generation
BM25/Citation lane only. After an approved BGE Activation, query-time Dense and
Rerank use only `siliconflow_bge_m3_v1`.

Python is not on the query-time retrieval path. It uses the scoped Go provider
gateway only for background SiliconFlow `retrieval.passage` batches and MinerU
jobs.

## 2. Signatures

```sql
knowledge_resolve_active_retrieval_profile()

knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
)

knowledge_fetch_fenced_profiled_lexical_evidence_candidates(
  UUID, UUID, UUID[], TEXT, INTEGER
)
```

```go
type RetrievalProfileGateway interface {
    EmbedQuery(context.Context, string) (QueryEmbedding, error)
    Rerank(context.Context, string, []string) ([]RerankResult, error)
}
```

The Go `ProviderGateway` creates an executable retrieval gateway only for the
immutable SiliconFlow profile. The unbound `EmbedQuery` and `Rerank` methods
fail closed so callers cannot accidentally use BGE against a historical Jina
Generation. No browser or Python caller supplies an upstream URL, header,
model, task, or credential.

## 3. Contracts

- Query input is trimmed, non-empty, and at most 2048 UTF-8 bytes.
- A historical Jina or legacy binding never triggers a provider request. It
  uses the exact bound Generation/Search Profile BM25 lane, then the normal
  hydration and Citation authority path.
- A BGE binding pins `Pro/BAAI/bge-m3`, exactly 1024 finite non-zero float32
  components, and `Pro/BAAI/bge-reranker-v2-m3`. Matching dimensions never
  authorize Jina/BGE vector reuse.
- Passage chunks use the matching SiliconFlow private gateway route with the
  infrastructure internal token. Go resolves the reusable credential; Python
  receives neither Key nor provider URL.
- There is no query/rerank URL environment setting and no Go-to-Python-to-Go
  provider cycle. Missing, disabled, corrupt, or unattested SiliconFlow state
  degrades only to the same BGE Generation/Profile BM25 lane where policy
  admits it.
- Postgres returns reference fields and rank only. Source-body hydration
  remains a later Go reauthorization step.
- Query vector, hybrid reader, BM25 fallback, diagnostics, and Rerank carry one
  immutable Generation/Search Profile binding. A concurrent pointer change
  raises `RAG_RETRIEVAL_PROFILE_CHANGED`; Go retries the complete bind/embed/
  read flow once.
- Dense candidates require at least eight trimmed query characters and cosine
  `>= 0.48`, then merge with keyword/CJK candidates through deterministic RRF
  `k=60`.
- Go globally fuses at most 20 references, authorizes provider-specific
  Rerank consent for the exact evidence Generation, and hydrates/reauthorizes
  source text in bounded `16 + 4` batches before source text leaves Go.
- Rerank resolves from the evidence Generation, not a later Active read. It
  sends the rewritten standalone query when present, otherwise the original,
  plus at most 20 authorized documents. The request sets
  `return_documents=false` and `top_n` to the document count. A response may
  omit `document` or return JSON null; any document body is rejected.
- A successful Rerank response returns every input index exactly once with a
  finite score in `[0,1]`. Go applies the versioned v2 ranking policy after
  Rerank: when the normalized basename of an authorized source file is
  explicitly present in the original or rewritten query, that document gets a
  deterministic metadata-only rank boost and its Children retain BGE score
  order. The filename never becomes quoted evidence or Citation authority.
  Go then sorts globally across all selected collections and injects at most
  five chunks.
- The provider client ignores environment proxies, follows no redirects,
  requires TLS 1.2 or newer, accepts identity-encoded JSON only, and bounds the
  response before decoding.
- Rerank and Answer are query-time collection authorities. Their consent
  changes do not advance `collection_processing_revision`; Parse or Passage
  Embedding consent changes still do.
- Jina plugin ID, Jina hostnames, retired RAG provider records, and historical
  Jina credentials are rejected before decryption or network access.

## 4. Validation & Error Matrix

| Condition                                                       | Result                                         |
| --------------------------------------------------------------- | ---------------------------------------------- |
| Query empty, oversized, or invalid                              | reject before provider access                  |
| Active/legacy binding is Jina                                   | same-Generation BM25 only; zero Jina requests  |
| SiliconFlow absent, disabled, unattested, or Vault unavailable  | same-BGE-profile BM25 where admitted           |
| Generation/Profile or retrieval pointer changes after Embedding | retry complete bind/embed/read once            |
| Vector is not 1024, finite, and non-zero                        | reject before Postgres                         |
| BGE vector is paired with a Jina/history binding                | reject as profile changed/invalid              |
| Hybrid Postgres query fails                                     | dependency error; do not disguise as a miss    |
| Candidate fails visibility/deletion/authority fences            | omit reference                                 |
| Hydration reauthorization fails                                 | omit evidence / follow Auto degradation policy |
| Rerank profile resolver or exact consent is absent              | send no source text; retain bounded RRF order  |
| Rerank provider/response is unavailable or invalid              | retain RRF Top5; `rerankStatus=degraded`       |
| Retired Jina plugin/provider/hostname is requested              | fail closed before secret access/network       |

No error or log may contain the query, Vault envelope, reusable provider Key,
provider response body, or source text.

## 5. Good / Base / Bad Cases

- Good after approved Activation: a Chinese semantic paraphrase crosses the
  BGE Dense gate, is reranked by the bound BGE model, and returns an authorized
  Citation.
- Base before Activation: the current historical Jina Generation returns an
  exact keyword hit through fenced BM25 with zero Embedding/Rerank calls.
- Base after Activation: SiliconFlow is unavailable, so the same BGE
  Generation/Profile lexical hit remains answerable with a Citation.
- Bad: a BGE query vector is applied to any Jina projection because both are
  1024-dimensional, or a Jina credential/adapter is consulted as fallback.
- Bad: governance is absent, the Generation resolver is unavailable, or a
  Rerank result index is duplicated/missing/non-finite; no unauthorized text
  leaves Go.

## 6. Tests Required

- Go unit: exact SiliconFlow URL/header/model shape, credential resolution,
  redirect rejection, bounded response decoding, vector validation, hybrid
  selection, same-profile BM25 fallback, Rerank validation, and visible DB
  failure.
- Retirement unit: old Jina Active bindings use lexical-only retrieval;
  retired plugin IDs/hostnames and provider credentials produce zero decrypt
  and HTTP calls; redirects to `jina.ai` or subdomains are rejected.
- Python unit: query-Embedding/Rerank HTTP paths remain absent; passage
  Embedding uses only the scoped Go DTO/internal token; provider capture cannot
  select or network to Jina.
- Postgres integration: migration `050` purges the Jina secret/plugin row,
  leaves the existing Jina Active row unchanged, keeps Candidate 8
  `verified/ready`, and rejects every later transition into Jina Active.
- Live proof before Activation: keyword Knowledge retrieval succeeds through
  BM25 while network/log capture contains zero `api.jina.ai` and
  `r.jina.ai` requests.
- Live proof after explicit Activation only: BGE semantic positive returns a
  finite vector and Citation; provider degradation remains same-profile BM25.

## 7. Wrong vs Correct

Wrong: treat the historical Jina schema tuple as executable, restore a deleted
Jina Key, use BGE against Jina rows, or compare against Jina during Candidate
promotion.

Correct: preserve historical rows for audit, serve the transition window with
fenced BM25, execute provider work only through the immutable SiliconFlow
profile, and keep Activation behind a separate explicit owner decision.

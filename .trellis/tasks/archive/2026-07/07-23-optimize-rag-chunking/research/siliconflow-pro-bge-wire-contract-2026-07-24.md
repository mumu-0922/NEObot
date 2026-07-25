# SiliconFlow Pro BGE wire contract (2026-07-24)

## Frozen profile

- Provider: `siliconflow`
- Embeddings endpoint: `https://api.siliconflow.cn/v1/embeddings`
- Embedding model: `Pro/BAAI/bge-m3`
- Embedding dimensions: fixed `1024`
- Maximum input: `8192` model tokens per input
- Maximum documented batch size: `32` inputs
- Encoding: `float`
- Query/passage instruction policy: none; the classic BGE API exposes no
  task/instruction field
- Rerank endpoint: `https://api.siliconflow.cn/v1/rerank`
- Rerank model: `Pro/BAAI/bge-reranker-v2-m3`
- Rerank `return_documents`: `false`
- Rerank `top_n`: exact input document count so response completeness can be
  verified
- Rerank server chunking controls are omitted; Neo Chat sends already bounded
  Child evidence and keeps its own source boundaries authoritative

The BGE-M3 endpoint does **not** support `dimensions`; only the Qwen3 embedding
series supports that request field. A BGE vector is validated as 1024 finite
components with a non-zero norm after response decoding.

## Embedding request/response

Request:

```json
{
  "model": "Pro/BAAI/bge-m3",
  "input": ["first text", "second text"],
  "encoding_format": "float"
}
```

Response authority:

- top-level required fields: `object`, `model`, `data`, `usage`
- each `data` item: `object`, `embedding`, `index`
- data items may be reordered; `index` must form the exact unique set
  `[0, input_count)`
- returned `model` must equal `Pro/BAAI/bge-m3`

## Rerank request/response

Request:

```json
{
  "model": "Pro/BAAI/bge-reranker-v2-m3",
  "query": "query",
  "documents": ["first", "second"],
  "top_n": 2,
  "return_documents": false
}
```

Response authority:

- top-level required fields: `id`, `results`; `meta` is optional
- each result contains the original input `index` and `relevance_score`
- because the request asks for all documents, indexes must form the exact
  unique input index set
- the documented response does not return a model ID; model identity is bound
  by the fixed endpoint adapter and request body, not inferred from the response

## Failure policy

- 400/401/403/404/429/503/504, non-JSON, encoded, oversized, malformed,
  duplicate/missing indexes, non-finite vectors/scores, and zero-vector
  responses fail closed with redacted stable errors.
- Provider response bodies, query text, document bodies, API Keys, and trace
  IDs are not returned in errors.
- SiliconFlow credentials use a dedicated `RAG:SILICONFLOW` vault record.
- No Jina query vector may search a BGE projection, and no BGE query vector may
  search a Jina projection, even though both are 1024-dimensional.

## Official references

- <https://docs.siliconflow.cn/cn/api-reference/embeddings/create-embeddings>
- <https://docs.siliconflow.cn/cn/api-reference/rerank/create-rerank>


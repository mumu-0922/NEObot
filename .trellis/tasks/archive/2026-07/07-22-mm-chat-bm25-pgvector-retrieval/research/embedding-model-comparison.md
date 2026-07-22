# Jina v4 versus BGE-M3

## Jina Embeddings v4

- Multimodal and multilingual model based on Qwen2.5-VL-3B-Instruct.
- Supports text, images, and visual documents; Dense and late-interaction
  retrieval; retrieval/text-matching/code adapters.
- Maximum sequence length 32768.
- Dense output is 2048 dimensions by default with Matryoshka dimensions
  including 1024, which matches the current mm-chat contract.
- mm-chat already distinguishes `retrieval.passage` and `retrieval.query` and
  uses the hosted Jina API, avoiding local inference load.

## BGE-M3

- Open-source 1024-dimensional multilingual model with up to 8192 tokens.
- Supports Dense, learned Sparse lexical weights, and ColBERT multi-vector
  outputs and advertises 100+ languages.
- Its learned Sparse lane is not the same as standard BM25.
- A single pgvector `vector(1024)` column only covers the Dense output; Sparse
  and ColBERT require additional storage/query designs.
- Self-hosting adds material RAM/CPU/GPU requirements to a constrained VPS.

## Published comparison caveat

The Jina v4 technical report reports higher averages than BGE-M3 on its
multilingual retrieval and LongEmbed tables, but it is a vendor-authored
benchmark and does not settle quality on the user's corpus. A frozen local
Golden Set is the promotion authority.

## Compatibility

Both profiles can produce 1024-dimensional vectors, but their vector spaces are
not compatible. Switching models requires re-embedding every passage and every
query under a separate immutable profile/generation; vectors must never be
mixed or backfilled by dimension alone.

## Recommendation

Retain `jina-embeddings-v4` and `jina-reranker-v3` for the BM25/pgvector
production migration. This permits valid existing Jina `REAL[]` vectors to be
converted into pgvector values without spending passage-embedding quota. After
the storage cutover is stable, benchmark BGE-M3 as a separate shadow generation
and promote it only if the same Golden Set proves a material quality/cost win.

## Sources inspected

- `https://huggingface.co/jinaai/jina-embeddings-v4`
- `https://arxiv.org/abs/2506.18902`
- `https://huggingface.co/BAAI/bge-m3`
- `mm-chat/backend/internal/ragproviders/provider_gateway_jina.go`
- `mm-chat/backend/internal/chat/rag_assembly.go`

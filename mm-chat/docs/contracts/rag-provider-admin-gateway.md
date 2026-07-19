# RAG Provider Administrator and Gateway Contract

## 1. Scope / Trigger

G11.9F.4 moves the MinerU and Jina credentials out of deployment environment
variables and the Python RAG process. It covers administrator configuration,
Postgres/vault persistence, bounded connection tests, runtime resolution, and
the private Python-to-Go provider boundary.

This gate does not redesign parsing or retrieval. MinerU remains the PDF
parser for scanned and formula-heavy documents. Jina remains
`jina-embeddings-v4` at 1024 dimensions plus `jina-reranker-v3`.
The legacy frontend MinerU BYOK/manual-parse path remains isolated during G7
and is not an input to automatic indexing; its retirement stays in G9.

## 2. Signatures

Administrator routes:

```text
GET    /v1/admin/rag/providers
PUT    /v1/admin/rag/providers/{provider}
DELETE /v1/admin/rag/providers/{provider}
POST   /v1/admin/rag/providers/{provider}/test
POST   /v1/admin/rag/providers/{provider}/activate
```

`provider` is exactly `mineru` or `jina`. Reads expose `hasApiKey`, enabled
state, connection-test state, and the fixed public model profile, but no
plaintext, browser envelope, vault envelope, or upstream response body.

Private provider routes:

```text
POST /internal/rag/providers/mineru/allocate
POST /internal/rag/providers/mineru/poll
POST /internal/rag/providers/jina/embeddings
```

The closed private DTOs are:

```text
allocate request  { filename }
allocate response { batchId, filename, uploadUrl }
poll request      { batchId, filename }
poll response     { batchId, filename, state, resultUrl? }
embedding request { passages: [{ passageId, text }] }
embedding response{ model, dimensions, vectors: [{ passageId, embedding }] }
```

MinerU filenames and batch IDs use bounded safe identifier grammars. Passage
IDs are non-zero UUIDs; one embedding call admits at most 256 unique passages,
64 KiB per passage, and 4 MiB of source text. Allocate/poll request bodies are
bounded to 8 KiB and the embedding request body to 5 MiB. Unknown fields are
rejected; caller-supplied URL, method, header, provider, model, task, Key, or
generic operation controls return `RAG_PROVIDER_OPERATION_UNSUPPORTED` before
provider resolution.

The private routes require `X-MM-Chat-Internal-Token` with the existing
`RAG_SOURCE_GATEWAY_TOKEN`. They accept closed, bounded JSON DTOs and never
accept an upstream URL, HTTP method, header, model ID, credential, or generic
operation name from Python.

Go directly owns Jina `retrieval.query` embedding and reranking for Knowledge
retrieval. Python calls the private Jina route only for `retrieval.passage`
worker batches. After cutover the old Go-to-Python
`/internal/retrieval/query-embedding` and `/internal/retrieval/rerank` provider
hop is removed, so no Go -> Python -> Go request cycle remains.

## 3. Contracts

- `provider_configs` stores exactly two reserved records per user:
  `RAG:MINERU` and `RAG:JINA`, with `config.kind="rag"` and the matching
  `config.ragProvider`. Model, Search, and RAG readers reject each other's
  kinds and reserved IDs.
- MinerU and Jina are complementary, not alternatives. Each may be activated
  independently; RAG provider status is fully ready only when both exact
  records are enabled and have valid connection attestations.
- administrator browser Keys cross the public boundary only through BYOK
  ingress contexts `provider:rag:mineru` and `provider:rag:jina`. Go
  immediately re-encrypts them under
  `provider:rag:<userId>:<RAG:MINERU|RAG:JINA>` and persists only the vault
  envelope.
- the legacy manual-parse MinerU BYOK never populates, overrides, or falls back
  into these administrator records or the G7 automatic-indexing resolver.
- save-and-test performs a real bounded test. Activation repeats the test and
  atomically commits the attestation and enabled state for the exact stored
  record. Key changes clear the proof and disable that provider.
- MinerU testing performs one fixed-name Local Batch allocation, validates one
  batch ID and one provider-signed upload capability, then discards both
  without uploading a document. Signed upload/result URLs returned during
  real jobs are validated, single-job capabilities; they are not reusable
  provider credentials and are never persisted or logged.
- Jina testing performs fixed, non-private sentinel calls against both
  `jina-embeddings-v4` and `jina-reranker-v3`; it requires a finite, non-zero
  1024-dimensional embedding and a valid rerank result.
- the attestation binds User, reserved record ID, provider, fixed endpoint and
  model profile, and the exact encrypted secret reference. It contains no Key.
- Go resolves an enabled, attested Postgres/vault record for every upstream
  call. Missing, disabled, corrupt, copied, or unattested state fails closed;
  there is no `.env` runtime fallback and no cross-provider fallback.
- Python receives neither the vault keyring nor MinerU/Jina Keys. It receives
  only bounded normalized results and, where required, a validated expiring
  MinerU job capability. The internal token grants only the three named
  operations; it cannot retrieve or lease a reusable provider credential.
- request/response size, item-count, timeout, redirect, content-type, numeric,
  model, and URL-host validation remain at least as strict as the current
  Python gateways. Go synthesizes every upstream URL and authentication header.
- the provider HTTP client ignores environment proxies, requires TLS 1.2 or
  newer, follows no redirect, accepts only identity-encoded JSON, and bounds
  MinerU/Jina response bodies to 1 MiB/16 MiB. MinerU upload and result
  capabilities are restricted to the frozen HTTPS hosts and path grammars.
- `RAG_SOURCE_GATEWAY_TOKEN` remains infrastructure configuration. Provider
  Keys are lazily imported only through an administrator save during the
  transition, then removed from Go/Python config, Compose, examples, and the
  operator env after the new path passes live activation and restart proof.

## 4. Validation and Error Matrix

| Condition                                                | Result                                      |
| -------------------------------------------------------- | ------------------------------------------- |
| Unknown provider, reserved-ID mismatch, malformed body   | `400 RAG_PROVIDER_CONFIG_UNSUPPORTED`       |
| Missing Key or unavailable vault                         | redacted `RAG_PROVIDER_SECRET_*` error      |
| Provider record absent                                   | `404 RAG_PROVIDER_NOT_FOUND`                |
| Bounded real test fails                                  | `502 RAG_PROVIDER_CONNECTION_TEST_FAILED`   |
| Stored state changes during a test                       | `409 RAG_PROVIDER_CONFIG_CHANGED`           |
| Provider disabled or attestation invalid                 | `409 RAG_PROVIDER_ACTIVATION_REQUIRED`      |
| Internal token absent or wrong                           | `401 RAG_PROVIDER_GATEWAY_UNAUTHORIZED`     |
| Unknown operation or caller-supplied upstream controls   | `400 RAG_PROVIDER_OPERATION_UNSUPPORTED`    |
| Invalid/oversized internal DTO                           | `400` or `413 RAG_PROVIDER_REQUEST_INVALID` |
| Upstream timeout, non-2xx, invalid or oversized response | redacted `502 RAG_PROVIDER_UPSTREAM_FAILED` |
| Database, vault, or resolver unavailable                 | `503 RAG_PROVIDER_UNAVAILABLE`              |

Provider Keys, vault envelopes, signed URLs, upstream response bodies, source
text, embeddings, and DNS/network details never enter errors or logs.

## 5. Good / Base / Bad Cases

- Good: save and activate both records, restart Go and Python without provider
  Key environment variables, auto-index one scanned PDF through MinerU and
  Jina, then retrieve its 1024-dimensional evidence through Go.
- Base: only Jina is active. Query/rerank can run, but PDF auto-index that
  requires MinerU fails with a stable provider-unavailable state rather than
  silently using a different parser profile.
- Bad: Python supplies `https://example.test`, `jina-reranker-v3`, or an
  `Authorization` header to the embeddings endpoint. The request is rejected;
  only Go chooses provider controls.
- Bad: copy the Jina vault envelope into `RAG:MINERU`, or alter a Key after
  testing. Context/attestation validation fails and no upstream call occurs.
- Bad: expose a `/credential`, `/token`, or generic HTTP proxy operation to
  Python. No such route exists; scoped normalized operations are the boundary.

## 6. Tests Required

- fixed record IDs/kind/provider validation, ownership, empty-array JSON,
  redacted reads, soft deletion, and separation from model/Search APIs;
- BYOK ingress contexts, RAG vault contexts, context-copy rejection,
  retained-key rotation, ciphertext-only backup/restore, and fresh-process
  reload;
- MinerU allocate probe and Jina embedding-plus-rerank test fixtures, real-test
  failure, zero activation on failure, fingerprint invalidation, concurrent
  change fencing, and activation persistence;
- internal-token constant-time rejection, closed-operation routing, strict JSON,
  body/item/response/timeout bounds, no redirects, fixed upstream URL/model/
  authentication shapes, and no credential-return path;
- Go direct query embedding/rerank plus Python passage-embedding and MinerU
  allocate/poll clients, including retryable/permanent error mapping and no
  Go -> Python -> Go provider cycle;
- frontend administrator load, saved-Key state, replacement/clear,
  save-and-test, activation/deactivation, deletion, and concise accessible
  status without retaining plaintext, plus legacy manual-BYOK isolation;
- isolated Postgres integration followed by test-database deletion;
- owner-authorized real MinerU parse and Jina indexing/retrieval, provider Key
  rotation, backend/RAG-worker restart, removal of all provider Key env names,
  and retained rollback backup verification.

## 7. Wrong vs Correct

Wrong:

```python
JinaPassageEmbeddingGateway(os.environ["RAG_JINA_API_KEY"])
```

Wrong:

```text
POST /internal/provider-proxy { url, method, headers, body }
```

Correct:

```text
administrator BYOK -> Go -> Postgres/vault -> bounded activation
Python closed DTO + internal token -> Go scoped operation -> exact provider
Go runtime retrieval -> direct Jina adapter -> normalized evidence
```

The Python process may invoke a fixed provider operation, but only Go can
resolve and use the reusable credential.

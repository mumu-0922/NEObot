# G16 Server-Only RAG Settings Plan

Status: complete. G16 removes the retired browser-owned RAG control plane
from `mm-chat` and makes the visible Knowledge settings match the deployed
MinerU, Jina, Go, Python, and Postgres runtime.

## Owner decisions

- `mm-chat` supports server-owned Knowledge/RAG only; the former root project
  remains unchanged.
- A dedicated **Knowledge Service** settings tab remains. It is not merged into
  the LLM provider page.
- The page contains two fully expanded, vertically stacked administrator cards:
  MinerU and Jina.
- Browser RAG enablement, LlamaParse, Upstash, chunk size, and TopK controls are
  removed. Server code owns parsing, chunking, retrieval, and evidence limits.
- A submitted provider Key is tested against the real provider before it can
  replace the current record. Success atomically persists and activates it;
  failure preserves the prior working Key and attestation.
- The default page is concise: overall status plus the two actionable provider
  cards. Internal Postgres/RAG-worker detail appears only when it explains a
  fault.
- Readiness degrades by stage. Missing MinerU suspends PDF parsing without
  disabling existing/native Knowledge retrieval; missing Jina makes indexing
  and retrieval unavailable while ordinary chat degrades explicitly.
- A versioned browser migration removes only the retired `mm-chat` Local-RAG
  URL, Token, provider Keys, and tuning fields. It does not touch server vault
  records or the former root application's browser data.

## Slice plan

### G16.1 Contract and evidence lock

- Record the live/new-vs-old control-plane trace, owner decisions, executable
  contract, deletion inventory, verification matrix, and rollback boundaries.
- Change documentation only and commit it independently.

### G16.2 Atomic provider configuration and staged readiness — complete

- Add one bounded administrator operation that accepts an encrypted transient
  MinerU or Jina Key, validates and tests it, then stores and activates it only
  after success.
- Preserve the current active vault record on validation, provider, timeout, or
  concurrency failure.
- Publish capability-oriented status for PDF parsing, native indexing, and
  retrieval without leaking credentials.
- Prove the service/repository/handler behavior and the deployed provider status
  before committing the backend slice.

### G16.3 Knowledge Service page and truthful health — complete

- Replace the mixed `RAGSettings` page with the concise server-only page.
- Render full MinerU and Jina cards vertically with masked saved state,
  save-and-test/replace, and confirmed removal actions.
- Remove the global RAG switch and all LlamaParse, Upstash, Chunk, and TopK UI.
- Derive overall and deployment health from server provider/capability status,
  not the retired public-config booleans or browser secrets.
- Pass focused component/API tests, full frontend gates, and a deployed browser
  reload proof before committing the frontend slice.

### G16.4 Local-RAG retirement and cutover closure — complete

- Remove retired Local-RAG settings types, normalization, resolvers, service
  shims, local Knowledge indexing/query branches, and related translations and
  tests when no server/import caller remains.
- After the G16.3 client cutover, retire the older RAG administrator PUT/test/
  activate routes and service methods so only atomic configure, redacted list,
  and confirmed delete remain writable.
- Add a bounded persisted-settings migration that deletes the obsolete RAG
  fields without clearing unrelated user preferences.
- Verify staged provider degradation, server Knowledge upload/index/query,
  citations, browser-storage cleanup, restart persistence, clean-copy checks,
  and complete temporary-state cleanup before the final independent commit.

## Verification matrix

```text
Backend static/unit       gofmt, go vet ./..., go test ./...
RAG worker                ruff, mypy, pytest for any touched Python surface
Frontend                  format, lint, typecheck, focused tests, full tests, build
Atomic failure            invalid/transient Key leaves old active record unchanged
Atomic success            real test passes, new record is active after restart
MinerU-only fault         PDF unavailable; Jina-backed existing/native retrieval works
Jina fault                indexing/retrieval unavailable; ordinary chat degrades visibly
Browser cutover           obsolete RAG fields absent; unrelated preferences retained
Live Knowledge            upload -> index -> selected-chat retrieval -> citation
Repository boundary       former root unchanged; each G16 slice committed separately
```

## Rollback

- Each G16 slice is a separate commit and can be reverted without reverting the
  other owner-parity work.
- G16.2 adds a bounded configuration operation over the existing provider
  records; failed tests perform no record mutation. Reverting it leaves the
  prior CRUD/test/activate endpoints and current encrypted records intact.
- G16.3 can be reverted independently while G16.2 remains backward compatible.
  Restoring the old page does not restore the already retired Next RAG routes.
- G16.4 browser cleanup is intentionally one-way: purged Local-RAG secrets are
  not recoverable from `mm-chat`. The owner explicitly chose deletion without
  export. Server-vault MinerU/Jina records, Knowledge documents, embeddings,
  citations, conversations, and former-root storage are outside that purge.

## Executable contract

### 1. Scope / trigger

This contract applies when an administrator views or changes Knowledge provider
credentials, when runtime health is projected to the browser, and when a
persisted pre-G16 settings snapshot is hydrated in server mode.

### 2. Signatures

```text
GET    /v1/admin/rag/providers
POST   /v1/admin/rag/providers/{mineru|jina}/configure
DELETE /v1/admin/rag/providers/{mineru|jina}
GET    /v1/rag/provider-status

configure request:
  { "apiKeySecret": <browser-encrypted secret envelope> }

provider status additions:
  status: ready | partial | unavailable
  capabilities.pdfParsing: boolean
  capabilities.nativeIndexing: boolean
  capabilities.retrieval: boolean
```

### 3. Request, response, and persistence

- `configure` accepts exactly one bounded browser-encrypted Key envelope for the
  path provider. Plaintext Keys are never persisted or returned.
- Go decrypts the transient envelope in memory, performs the provider-specific
  bounded test, and only then encrypts the Key with the server vault and commits
  the enabled record plus valid connection attestation.
- A failed test, timeout, malformed response, stale concurrent update, or
  database failure leaves the prior record byte-for-byte authoritative.
- MinerU and Jina records remain fixed provider identities; the operation cannot
  create arbitrary RAG providers.
- Browser hydration discards the retired Local-RAG object fields. It never
  imports them into Postgres and never deletes server provider records.

### 4. Validation and error matrix

```text
unknown provider / unknown JSON field       -> 400 RAG_PROVIDER_CONFIG_UNSUPPORTED
missing or malformed encrypted Key          -> 400 RAG_PROVIDER_SECRET_REQUIRED/INVALID
server vault unavailable                    -> 503 RAG_PROVIDER_SECRET_VAULT_UNAVAILABLE
real provider authentication/test failure   -> 502 RAG_PROVIDER_CONNECTION_TEST_FAILED
provider record changed during operation    -> 409 RAG_PROVIDER_CONFIG_CHANGED
database unavailable                        -> 503 DATABASE_REQUIRED
successful tested configuration             -> 200, enabled=true, attestation valid
```

### 5. Good, base, and bad cases

- Good: a replacement Jina Key passes the bounded v4 embedding/rerank test, is
  atomically stored and activated, survives restart, and never appears in a
  response or log.
- Base: both current records are ready; opening/reloading the page performs only
  redacted reads and shows two enabled cards.
- Partial: MinerU is missing while Jina is ready; existing/native Knowledge
  retrieval remains available and PDF parsing is reported unavailable.
- Bad: an invalid MinerU Key fails its real test; the previously active MinerU
  record remains ready and subsequent PDF work still resolves it.

### 6. Required tests

- Service and repository tests assert test-before-commit, prior-record
  preservation, activation attestation, restart reload, and concurrency fencing.
- Handler tests assert strict JSON/path handling, status/error mapping, no-store
  reads, and redacted responses.
- Capability tests cover ready, MinerU-missing, Jina-missing, and both-missing
  matrices plus exact stage admission behavior.
- Frontend tests assert two vertical cards, no retired controls/strings, atomic
  client calls, failure state, confirmed removal, truthful health, and storage
  migration preservation.
- Live proof covers provider status, one bounded real provider test, a failed
  replacement that preserves the old record, browser reload, Knowledge
  upload/index/retrieval, and cleanup.

### 7. Wrong vs correct

```text
Wrong:
  one page = server provider admin + browser RAG toggle + LlamaParse + Upstash
  save bad Key -> overwrite working record -> test fails -> runtime broken

Correct:
  Knowledge Service = MinerU card + Jina card + server capability status
  transient Key -> real test -> atomic vault commit/activate -> runtime resolver
  selected chat Knowledge -> Go/Python/Postgres RAG; no browser RAG authority
```

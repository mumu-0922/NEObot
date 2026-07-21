# G16 Server-Only RAG Settings Process

## 2026-07-21 — G16.1 runtime trace and owner lock

The deployed settings page combined two unrelated control planes. The original
root `RAGSettings` page owned a browser-local pipeline: a global enable switch,
MinerU/LlamaParse parsing, browser chunk-size and TopK values, and an Upstash
Vector URL/Token. Commit `608ef00` inserted `RAGProviderAdmin` into the copied
page without replacing that legacy surface.

The standalone runtime no longer executes the browser pipeline. Server-mode
Knowledge selects `ServerKnowledgeBase`, uploads and binds files through Go
`/v1/files` and `/v1/knowledge`, routes PDFs to MinerU and native formats to the
native parser, uses Jina 1024-dimensional embeddings/reranking, and stores the
search projection in Postgres. G9.2 removed the old Next `/api/rag/*`,
`/api/doc-parse/*`, and query-rewrite routes; their remaining frontend service
entrypoints fail closed.

Read-only live evidence before any G16 code change:

```text
GET /v1/config RAG flags                  vector=false; document=false
GET /v1/rag/provider-status               MinerU ready; Jina ready/1024; ready=true
GET /v1/admin/rag/providers               both enabled, keyed, attested
old Local-RAG route handlers              removed in G9.2
old ragService/docParseService            fail-closed compatibility shims
provider calls / quota used by trace       none
mm-chat working tree before G16.1          clean
```

This explains the contradictory screenshot: the header said RAG was disabled
because it read obsolete browser state while the real server provider chain was
ready. The LlamaParse, Upstash, Chunk, and TopK controls could not affect server
Knowledge, and Deployment Health could also report the old state.

Owner decisions locked through the G16 grill:

1. Retire Local RAG completely from `mm-chat`; do not modify the former root.
2. Keep a dedicated **Knowledge Service** settings tab.
3. Keep server parsing/retrieval parameters code-owned and absent from the UI.
4. Test a new Key against the real provider before atomically replacing and
   activating it; preserve the prior Key on failure.
5. Show concise, actionable status and reveal internal infrastructure only on
   faults.
6. Render full MinerU and Jina cards, always visible and stacked vertically.
7. Degrade by stage instead of disabling all Knowledge for a MinerU-only fault.
8. Remove obsolete browser Local-RAG credentials and parameters without export.

G16.1 changes documentation only. The implementation proceeds in three bounded,
independently tested commits: backend atomic configuration/status, frontend
server-only settings/health, then legacy code and browser-state retirement.

G16.1 verification:

```text
git diff --check -- mm-chat                                      passed
pnpm prettier --check G16 plan/process + tracking progress       passed
runtime or provider mutation                                     none
```

## 2026-07-21 — G16.2 atomic provider configuration and staged status

The former administrator flow persisted a replacement Key first, tested the
stored record second, and activated it in a third request. A failed replacement
therefore left the bad Key stored and the prior working vault record lost even
though activation correctly remained false.

G16.2 adds `POST /v1/admin/rag/providers/{provider}/configure`. Go decrypts the
bounded browser envelope in memory, runs the fixed MinerU allocate probe or the
Jina embedding-plus-rerank probes, encrypts the tested Key with the provider
vault, and calls one serializable repository operation. That transaction locks
the provider table, compares the pre-test record snapshot, and either creates
or replaces the record with `enabled=true` and a matching attestation. Test,
vault, database, or concurrency failure leaves the old active row unchanged.

`GET /v1/rag/provider-status` now retains the compatibility `ready` boolean and
also returns `status` plus `pdfParsing`, `nativeIndexing`, and `retrieval`
capabilities. Both providers ready is `ready`; Jina ready without MinerU is
`partial`; missing/unavailable Jina makes indexing/retrieval `unavailable`.
Existing runtime adapters already resolve the exact provider per stage, so no
global provider-status gate was introduced.

Verification:

```text
targeted ragproviders/runtimeconfig/httpserver tests              passed
backend gofmt / go vet ./... / go test ./...                      passed
targeted Go race tests                                             passed
isolated Postgres create -> replace -> stale fence                passed
isolated database mm_chat_g16_atomic_test after proof             absent
source-built backend recreate / health                            healthy
live status                                                       ready; all capabilities true
live stored MinerU real allocate test                              passed
live invalid atomic MinerU replacement                             502 expected
active MinerU redacted record before/after failed replacement     byte-identical
status after failed replacement                                   ready
invalid replacement value in backend logs                         zero hits
change / quality / changed-scope security gates                    passed / passed / passed
```

The positive live probe reused the already encrypted server record and changed
only its connection-test timestamp/attestation. The negative configure probe
used a disposable invalid browser envelope, consumed no document upload, did
not replace the record, and left no temporary local file or test database.

## 2026-07-21 — G16.3 server-only page and truthful health

The copied settings surface still rendered the retired browser RAG control
plane beside the server provider administrator. G16.3 replaces it with one
Knowledge Service header, capability status, and two always-visible vertical
cards for MinerU and Jina. Each card accepts a transient browser-encrypted Key,
uses the G16.2 atomic configure operation, clears the plaintext draft after
success, and requires a second click before deleting a stored Key.

The page no longer renders the global enable switch, LlamaParse, Upstash,
browser chunk size, or TopK. Its initial and post-mutation reads use the
redacted provider list plus `/v1/rag/provider-status`. Deployment Health now
uses the same server status: `ready` is healthy, MinerU-only `partial` is a
warning, configured-but-unavailable is blocked, and fully unconfigured is
missing. It no longer treats public-config RAG flags or browser credentials as
runtime evidence.

Verification:

```text
frontend Prettier / ESLint / TypeScript                         passed
focused API/settings/health tests                              67 passed
full frontend Vitest                                           191 files / 912 tests passed
frontend production build                                     passed
Compose source build + frontend recreate                       passed / healthy
fresh Windows Chrome Knowledge Service reload                  ready
visible provider cards                                         MinerU + Jina, vertical
retired visible controls                                       absent
fresh Windows Chrome Deployment Health                         Knowledge service healthy
live provider list/status reads                                200 / 200
provider Key mutation or real provider quota                   none
temporary screenshots/profiles                                 removed
```

G16.3 changes only the `mm-chat` frontend and tracking documents. The older
client/backend PUT, test, and activate compatibility methods deliberately
remain until G16.4 removes their last code and test references in one bounded
cutover.

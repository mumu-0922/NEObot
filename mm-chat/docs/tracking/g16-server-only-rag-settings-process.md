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

## 2026-07-21 — G16.4 Local-RAG retirement and cutover closure

The final slice removed the browser-owned RAG runtime rather than leaving a
second inactive implementation behind. The frontend now has only Server
Knowledge collections, current-chat collection selection, Go upload/bind,
explicit legacy-browser import, and citation display. Local indexing/query
services, the local Knowledge store and hydration hook, document-parse job
client, Local-RAG settings/secrets/types, Workspace Knowledge binding, query
generation, compatibility translations, and their tests were deleted. The
backend also retired the older provider PUT/test/activate write sequence and
removed the obsolete public-config `rag` projection. The remaining contract is
redacted list, atomic configure, confirmed delete, and capability status.

A versioned startup migration checks both `localStorage` and the
`neo-chat/app_data` IndexedDB store. It removes only top-level `rag` and
`state.rag`, writes a completion marker after both sources parse, and preserves
all other values. Four unit cases cover both stores, idempotency, no-op input,
and parse-failure atomicity. A fresh dedicated Windows Chrome profile then
proved the deployed migration: both obsolete objects disappeared, the marker
became `1`, and injected theme, Search, Voice, Plugin, and unrelated sentinel
state stayed byte-for-byte equivalent. The dedicated browser process, script,
output, and profile were removed afterward.

The real Knowledge proof exposed one additional cross-layer defect before
closure. A Server Default chat carries `ModelRef.providerId=SERVER_DEFAULT`,
while server-provisioned answer consent is keyed by the authoritative provider
processor such as `openai_compatible`. Retrieval therefore found evidence but
answer governance rejected it as `answer_governance_required`. The runtime
provider resolver now returns a separate server-derived answer processor for
governance only. The actual provider ModelRef remains unchanged; browser input
cannot choose the consent identity. Canonicalization is shared with startup
consent provisioning, and focused handler/runtime tests cover Server Default,
server-stored, and browser-supplied provider boundaries.

Live proof after rebuilding and restarting the backend:

```text
provider status after restart                              ready
MinerU / Jina                                              ready / ready, 1024
native text upload -> Jina index                           processing -> active
selected-chat hit                                          ORCHID-7429-ZETA [K1]
hit metadata                                               answered, citationCount=1
answer governance                                          openai_compatible/server-default
rerank                                                     applied
unrelated Nimbus Harbor query                              no_evidence
miss metadata                                              citationCount=0
temporary conversations/document/collection/file           deleted; subsequent reads 404
temporary collection in list                               absent
Postgres databases after cleanup                           neo_chat, postgres only
```

The delete contract is immediate invisibility, not destructive audit-history
erasure. Post-delete inspection confirmed collection/document/file tombstones
remain as designed, while their API reads return `404` and retrieval cannot
reauthorize the retained derived projection rows. No manual production-table
deletion was used to make the proof pass.

The final staged-reference audit caught a second, non-runtime residue layer:
unused browser vector splitters, Local-RAG/document-parse request schemas,
LlamaParse URL/BYOK constants, and the obsolete `DOCUMENT_PARSE_JOB_STORE`
deployment projection. Those files, tests, environment/Compose wiring, health
fields, and stale inventory references were removed before commit. The Go
public-config regression now asserts that neither `rag` nor
`documentParseJobStore` can reappear in `/v1/config`.

Two runtime recovery notes were captured during verification. An interrupted
earlier Compose build had produced a zero-byte backend binary and `exec format
error`; a complete no-cache rebuild restored a non-empty binary and healthy
container. Later, one rebuild omitted `--env-file .env.single-server`, causing
the recreated backend to use the Compose default `AUTH_MODE=required`. No
volume was deleted. Recreating with the explicit env file restored the intended
development profile and all persisted provider/document state. All subsequent
Compose commands used the explicit env-file boundary.

Final verification:

```text
retired backend PUT                                       405, Allow: DELETE
retired backend /test and /activate                       404 / 404
/v1/config legacy rag field                               absent
backend gofmt / go vet ./... / go test ./...              passed
frontend Prettier / ESLint / TypeScript                   passed
frontend Vitest                                           185 files / 882 tests passed
frontend production build                                 passed
isolated standalone structure gate                        passed
isolated full clean-copy gate                             passed
clean-copy Go                                             vet + all tests passed
clean-copy RAG                                             Ruff/Mypy; 1730 passed, 7 skipped
Compose backend/frontend                                  healthy / healthy
git diff --check -- mm-chat                               passed
former-root application                                   unchanged
```

No provider Key, browser secret, private document text beyond the disposable
sentinel, temporary browser profile, temporary test database, or live test
resource remains. G16 is complete; rollback remains the independent revert of
the four G16 commits.

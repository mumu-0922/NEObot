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

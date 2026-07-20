# G11.9 Auto Knowledge and Web-Augmented Chat Plan

## Goal

Replace the current per-message strict evidence gate with a ChatGPT-style
Auto Knowledge experience: selected Knowledge bases persist for the current
conversation, relevant private evidence augments rather than replaces the
model, optional Web Search can contribute current public evidence, and every
used source remains attributable.

This plan is the implementation authority for G11.9. Work remains sliced: one
slice is implemented, tested, documented, and committed before the next begins.

## Frozen Product Decisions

- Knowledge has one user-facing mode: **Auto**. Remove the `STRICT` mode,
  `insufficient_evidence` refusal card, and frontend `ragStrict` /
  `knowledgeStrict` hard-coding.
- Selecting Knowledge binds up to eight collections to the current
  conversation. The selection survives messages, refresh, and navigation; a
  new conversation starts unbound.
- Every message performs bounded retrieval over the bound collections. Only
  evidence that passes relevance gates is injected; a normal miss is silent.
- A provider/system failure is distinct from a normal miss and produces a
  lightweight degraded-answer notice.
- Questions that explicitly request document-only answers remain ordinary
  natural-language instructions; there is no separate Strict product mode.
- Web Search is opt-in through the existing search toggle. When enabled, the
  Router decides whether current public evidence is useful and may derive
  queries from the question plus relevant Knowledge context.
- Source authority depends on the question: selected private Knowledge for
  internal facts, Web for current public facts, and the model for synthesis.
  Conflicts are shown rather than silently collapsed.
- Knowledge citations use `[K<n>]`; Web citations use `[W<n>]`. No source is
  fabricated for model reasoning.
- Do not add a user-visible Retrieval Testing page. Keep structured server
  diagnostics sufficient to reconstruct routing, recall, rerank, filtering,
  hydration, and degradation decisions without logging secrets or source text.
- Remove SearXNG. Migrate Tavily, Firecrawl, Exa, Bocha, and supported
  model-built-in search from the legacy Next route to Go.
- Multiple search providers may be configured, but exactly one is active.
  There is no automatic provider fallback in G11.9.
- Search-provider activation requires an explicit bounded real connection
  test. A failed configuration may be saved but cannot become active.
- External provider configuration and encrypted secrets are Postgres-owned.
  The sole encryption master key and infrastructure/internal credentials live
  outside Postgres in Docker Secrets; provider-specific `.env` fallbacks are
  removed after one-time import and verification.

## Target Runtime Flow

```text
message + recent conversation context
  -> conditional standalone-query rewrite
  -> original + rewritten query lanes
  -> selected Knowledge collections only
       keyword/CJK phrase recall
       Jina retrieval.query vector recall
       RRF fusion
       Jina reranker-v3
       global threshold + TopK
       current-authority hydration
  -> optional Web Router when search is enabled
       one active Go SearchProvider
  -> source-aware context assembly
  -> selected model generation
  -> [K] / [W] citations and structured diagnostics
```

## G11.9A — Development Hydration and Auto Semantics

Objective: make the already indexed and recalled owner document reach answer
generation, then remove strict refusal behavior without weakening current
hydration authorization.

Actions:

- provide development single-user requests with a server-owned, database-valid
  internal Session identity instead of only a User identity;
- bootstrap answer governance and owner/collection consent for every enabled,
  connection-tested backend-persisted model so changing models in the
  administrator UI does not silently disable Knowledge;
- prove candidate -> reauthorization/hydration -> answer-governance -> model
  context end to end against the real `test` collection;
- change selected-Knowledge handling from forced strict refusal to Auto:
  relevant evidence augments, normal no-evidence continues without a purple
  status card, and dependency failures surface as degradation metadata;
- retain citations only when evidence was actually injected.

Verification:

- Chinese question `研究方向是什么` hydrates the active DOCX evidence and answers
  with `[K1]`;
- unrelated question completes through the model without a Knowledge refusal;
- development Session identity cannot be selected by a browser Bearer token;
- hosted authenticated behavior and authorization rejection tests remain green.

Rollback: restore the prior chat decision branch and remove the internal
development Session bootstrap; migration/data changes must be independently
reversible.

## G11.9B — Conversation-Persistent Knowledge Binding

Objective: select Knowledge once per conversation instead of attaching it to
every message.

Actions:

- persist selected collection IDs in server conversation state/metadata;
- add a dedicated composer Knowledge control and persistent removable chips;
- support up to eight selected collections and no unselected collection access;
- keep new conversations unbound by default;
- migrate a current message attachment selection into conversation binding
  without retaining a second authority path.

Verification: selection survives refresh/navigation and subsequent messages,
removal takes effect on the next message, and a new conversation has no binding.

Rollback: retain read compatibility for existing message metadata while the
conversation binding write path is reverted.

## G11.9C — Contextual Hybrid Retrieval and Rerank

Objective: deliver production-quality multilingual retrieval.

Incremental execution slices:

- **G11.9C.1** conditional standalone-query rewrite, original+rewritten
  keyword/CJK lanes, deterministic RRF, global candidate 20 and Evidence 5;
- **G11.9C.2** complete: private Python query service, Jina
  `retrieval.query` 1024 Dense lane, Postgres hybrid candidate function,
  conservative Dense signal gates, and Go keyword degradation;
- **G11.9C.3** complete: private `jina-reranker-v3`, evaluated `>= 0.0`
  threshold, RRF-only degradation, query-time consent without index
  invalidation, real applied/failure/cross-collection proof, and final G11.9C
  promotion.

Actions:

- rewrite only context-dependent follow-ups using the recent four to six turns;
- search both original and rewritten queries and fall back if rewrite fails;
- add Jina `retrieval.query` 1024-dimensional query embeddings;
- combine keyword/CJK and vector candidates with RRF;
- retrieve a broad global candidate set (initially 20), rerank with
  `jina-reranker-v3`, apply an evaluated score threshold, and inject at most
  five chunks across all selected collections;
- degrade to hybrid pre-rerank ordering in Auto when rerank alone is unavailable.

Verification includes exact identifiers, Chinese paraphrases, conversational
follow-ups, unrelated negatives, rerank failure, and cross-collection TopK.

Rollback: feature-gate semantic/rerank lanes while retaining the current
reference-only keyword candidate and authorization boundary.

## G11.9D — Structure-Aware Parent/Child Reindex

Objective: replace the one-document/one-chunk Native baseline with useful
structure-aware retrieval units.

Incremental execution slices:

- **G11.9D.1** complete: pure deterministic structure-aware chunk planner,
  frozen Parent/Child/overlap bounds, UTF-8-safe source-range references, and
  no runtime generation mutation;
- **G11.9D.2.1** complete: map validated Native headings, paragraphs, lists,
  table rows, code, source positions, and exact planner ranges into schema-valid
  Canonical IR/Chunk Manifest artifacts plus Postgres projection DTOs, without
  changing the production gateway or active generation;
- **G11.9D.2.2** complete: map admitted MinerU page elements, headings, text,
  tables, formulas, page geometry, and clipped canonical anchors behind a
  MinerU structure profile, with schema and Postgres projection proof;
- **G11.9D.2.2a** complete: converge Native and MinerU manifests on one shared
  structure chunk profile required by mixed-format generation staging;
- **G11.9D.2.3a** complete: atomically allocate the one permitted non-active
  rebuild generation, shared Index/Search Profiles, and one staging
  materialization plus pending parse job for every current active document;
- **G11.9D.2.3b** complete: resolve the leased generation chunk profile, retain
  baseline routing for the active generation, route the shared candidate to
  Native/MinerU structure parsers, and live-stage one real PDF plus two DOCX
  projections while leaving all three passage-embedding jobs pending;
- **G11.9D.2.3c** complete: run the existing token-fenced passage-embedding
  handler against the shared candidate with real Jina 1024-dimensional
  `retrieval.passage` vectors, publish all three materializations, and prove
  exact candidate coverage/completeness without re-upload or active-head
  mutation;
- **G11.9D.2.3** complete: candidate allocation, mixed Native/MinerU structure
  parsing, real passage embeddings, and per-materialization completeness are
  closed;
- **G11.9D.3a** complete: lock the corpus head, prove exact generation-wide
  document/job/artifact/Block/Parent/Child/vector/locator completeness, freeze a
  deterministic manifest and counts, and transition only
  `building -> verified` / `building -> ready` with deterministic replay;
- **G11.9D.3b** complete: serialize delete/promotion on the corpus head,
  revalidate the verified candidate inside promotion, reject stale coverage,
  atomically and idempotently fail the candidate, and prove a replacement can
  be allocated while the old generation stays active;
- **G11.9D.3c** complete: grant the fenced cutover, atomically activate the
  verified structure generation, prove real Parent/Child citations, reject an
  incomplete rollback target, and restore the exact source generation;
- **G11.9D.3** complete: verification, deletion/concurrency failure fencing,
  atomic cutover, live citations, and guarded source-generation rollback are
  closed;
- **G11.9D** complete: the structure-aware generation was rebuilt from current
  files without re-upload, verified, activated, queried, cited, and rolled back
  on a disposable production-shape clone.

Actions:

- preserve headings, paragraphs, lists, table rows, and source locators;
- target child chunks around 300–500 tokens with about 50-token overlap and
  parent chunks around 1,200–2,000 tokens, adjusted at structural boundaries;
- build a new versioned Index Generation for existing active documents;
- re-embed automatically without requiring re-upload;
- verify the new generation before atomic cutover, keeping the old generation
  queryable until success.

Verification covers citation anchors, tables, multilingual boundaries,
generation failure rollback, and deletion visibility during rebuild.

## G11.9E — Go Web Search Providers

Objective: remove production dependence on legacy Next `/api/search`.

Actions:

- implement a closed Go `SearchProvider` contract for Tavily, Firecrawl, Exa,
  Bocha, and supported model-built-in search;
- port existing request/response normalization, bounded fetch, SSRF, timeout,
  response-size, and result-limit protections;
- remove SearXNG types, UI, configuration, tests, and documentation;
- keep multiple saved providers but one active provider per request;
- issue `[W]` citations from normalized source records;
- delete the legacy Next route only after Go adapter parity and live smoke.

Verification uses provider fixtures plus at least one owner-authorized real
provider call without automatic cross-provider fallback.

Incremental execution slices:

- **G11.9E.1** complete: add the closed Go Tavily/Firecrawl/Exa/Bocha adapter
  contract, shared HTTPS/DNS/IP/redirect/response bounds, redacted errors, and
  fixture-tested result normalization without a production route or Key use;
- **G11.9E.2** complete: add the server-owned active resolver, authenticated Go
  Search API, stable fail-closed execution errors, shared built-in source
  normalization, and explicit OpenAI Responses Web Search capability while
  rejecting built-in Search for OpenAI Compatible providers;
- **G11.9E.3** complete: cut the frontend to Go SSE, issue/persist bounded
  `[W]` citations and Search output blocks, delete the retired self-hosted and
  legacy Next Search chains, prove live Postgres reload, and pass authorized
  real-provider negative capability/error smokes without fallback. Positive
  credentialed activation remains G11.9F.

## G11.9F — Admin Provider Secrets and Connection Tests

Objective: make Postgres the sole authority for all external provider settings
while keeping the decryption root outside the database.

Actions:

- add administrator CRUD/activate/test contracts for model, search, MinerU,
  and Jina providers, while reserving a non-executable provider-kind/vault
  identity for future Voice providers;
- encrypt provider secrets with a Docker-Secret-backed master key;
- expose configured/not-configured state but never plaintext on reads;
- perform a bounded real connection test before activation for every
  implemented provider; keep the reserved Voice path fail-closed until its
  future provider-specific test and executor exist;
- one-time import current provider `.env` values, verify them, then remove
  provider-specific environment fallbacks;
- keep Python RAG workers from receiving the master key by using a scoped Go
  internal provider/credential proxy.

Verification covers restart persistence, key rotation, ciphertext-only backup,
redacted errors/logs, failed activation, and provider removal.

Incremental execution slices:

- **G11.9F.1** complete: add an unused, strict Docker-Secret keyring loader
  and context-bound AES-256-GCM at-rest vault with retained-key rotation tests;
  do not mutate Postgres, Compose, routes, runtime resolution, or current keys;
- **G11.9F.2.1** complete: mount a stable mode-`600` Docker Secret keyring for
  matching non-root backend/admin UIDs, re-encrypt new model-provider BYOK
  ingress before Postgres writes, dual-read legacy rows, and lazily import
  legacy/default env secrets on administrator metadata save;
- **G11.9F.2.2** complete: transactionally backfill every remaining
  model-provider row, rotate retained-key vault envelopes, reject unrecoverable
  custom legacy rows, and prove owner-only ciphertext backup/restore plus
  active-key-only restart;
- **G11.9F.2.3** complete: require a bounded model-provider connection test
  before activation, invalidate the proof on credential/type/endpoint change,
  resolve chat/image execution only from tested Postgres/vault state, and
  remove model-provider `.env` runtime fallback;
- **G11.9F.3** complete: the frozen
  [`Search Provider Administrator Contract`](../contracts/search-provider-admin.md):
  external Search CRUD/save-and-test/atomic activate UI, the sole
  Postgres/vault Go resolver, and automatic built-in Search only when no
  external provider is active and the selected model capability matches are
  implemented and locally deployed; owner-authorized Tavily Search, chat
  `[W]` persistence/reload, restart continuity, and no-fallback failure proofs
  passed with test artifacts cleaned;
- **G11.9F.4** complete: add MinerU/Jina administrator records and a scoped Go credential
  gateway so Python receives neither the vault keyring nor reusable secrets:
  - **G11.9F.4.1** complete: freeze the
    [`RAG Provider Administrator and Gateway Contract`](../contracts/rag-provider-admin-gateway.md),
    including reserved records, vault contexts, real-test semantics, the
    Python-to-Go operation allowlist, and environment-removal order;
  - **G11.9F.4.2** complete: add Postgres/vault MinerU/Jina administrator CRUD,
    save-and-test/activate UI, dynamic provider status, and transition-only
    administrator import of the existing environment Keys;
  - **G11.9F.4.3** complete: add the closed Go MinerU allocate/poll and Jina passage
    embedding gateway plus direct Go Jina query/rerank adapters, without
    cutting Python off its current provider environment path yet;
  - **G11.9F.4.4** complete: switch Python to the scoped Go operations, retire the old
    Go-to-Python Jina routes, and remove MinerU/Jina provider Key environment
    parsing, Compose wiring, examples, and operator values;
  - **G11.9F.4.5** complete: real scanned/complex-formula PDF parsing, Jina
    1024-dimensional indexing/query/rerank, Replay-login recovery, active-only
    keyring rotation with attestation preservation, paired backup/restore,
    rollback rehearsal, redaction, and destructive smoke cleanup all passed;
- **G11.9F.5** complete: reserve exact `VOICE:ELEVENLABS`/`VOICE:MIMO`
  provider-kind and context-bound vault identities, exclude them from other
  provider readers, rotate retained-key ciphertext without inventing a
  connection proof, block legacy BYOK Voice rows, reject old Voice env
  authority in production preflight, and keep `/v1/voice/*` fail-closed. The
  final active-only rotation, paired backup/restore, redaction, runtime-health,
  and clean-copy full gates passed without adding a Voice administrator route,
  UI, resolver, provider call, or VPS-local TTS runtime. G11.9F is closed.

## G11.9G — Knowledge/Web/Model Fusion Closure

Objective: prove the complete Auto product behavior.

Actions:

- route optional Web only when the user enabled search;
- combine relevant Knowledge and Web context with explicit source identities;
- make question-type authority and conflict presentation deterministic;
- persist `[K]` and `[W]` citation artifacts through stream completion/reload;
- emit structured diagnostics containing IDs, stages, timings, scores, selected
  provider, and degradation reason but no raw secrets or full source text.

Verification matrix:

- Knowledge only; Web only; both; neither;
- normal Knowledge miss; Knowledge dependency failure;
- Web disabled; Web enabled but unnecessary; Web provider failure;
- conflicting private/current public evidence;
- conversation reload and citation-card interaction;
- clean-copy source build and live provider/indexing/search smoke.

Incremental execution slices:

- **G11.9G.1** complete: freeze the source-fusion contract and add the deterministic
  question/authority Router so Search is never called when disabled and may be
  skipped when relevant Knowledge already answers a non-current question;
- **G11.9G.2** complete: run external Web only after Knowledge routing, derive a bounded
  query from the question plus admitted Knowledge, degrade provider failures to
  a normal model answer, and persist redacted stage/timing diagnostics;
- **G11.9G.3** complete: close model-built-in Search routing, deterministic conflict
  instructions, combined `[K]`/`[W]` persistence/reload, and frontend citation
  interaction parity;
- **G11.9G.4** complete: execute the full matrix, owner-authorized live
  Knowledge/Web/model smoke, restart/clean-copy verification, cleanup, and
  close G11.9;
- **G11.9G.5** complete: correct post-closure citation truth by retaining only
  final-answer `[K]` markers, recomputing terminal source authority from valid
  used `[K]`/`[W]` markers, and hiding stale pre-fix Knowledge cards on reload;
- **G11.9G.6** complete: prevent loosely related Knowledge from polluting an
  explicit-subject Web query while retaining bounded Knowledge derivation for
  genuinely context-dependent follow-ups.

G11.9 is complete. The final deployed proof used the active structure-aware
Knowledge generation, Jina query/rerank, Tavily, and `gpt-5.5`; every temporary
conversation and local smoke artifact was removed after reload verification.
The G11.9G.5 correction does not replace the evaluated rerank gate with one
unsafe global cutoff: live useful and unused candidates had overlapping score
ranges, while final citation-marker use separated the observed cases.

## Definition of Done

- Every G11.9 slice is implemented, verified, documented, and committed alone.
- The owner no longer reselects Knowledge for every message.
- Selecting Knowledge never forces a strict refusal mode.
- Relevant Chinese DOCX evidence reaches the model with a citation.
- Unrelated questions answer normally without an empty Knowledge status card.
- Go owns all production Search execution; SearXNG and legacy Next search are
  absent.
- Provider secrets have one encrypted Postgres authority and no provider `.env`
  drift path.
- Existing active documents are structure-aware and queryable after atomic
  reindex cutover.

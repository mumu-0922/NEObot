# G11.9 Auto Knowledge and Web-Augmented Chat Process

This log records one G11.9 slice at a time. Evidence must identify the exact
runtime state, inputs, provider calls, database effects, verification, rollback
surface, and commit before the next slice begins.

## 2026-07-17 — Requirements grill and live fault isolation

Owner-visible symptoms:

- Knowledge had to be attached again for every message;
- selecting it forced a purple `STRICT` refusal card;
- `研究方向是什么` refused even though the active DOCX contained the answer;
- Knowledge could not supplement model knowledge or optional Web Search.

Live isolation:

```text
conversation selected collection                 ec6e5c2d-dc7e-4e86-a805-5c912c413ae3 (test)
active document                                  e7845c02-8976-45d4-9753-617a2f0e1477
query candidate count: 研究方向是什么               1
persisted user metadata                          selected collection correct
persisted assistant outcome                      insufficient_evidence
decisive branch                                  auth.SessionFromContext absent
development middleware                           injected User only, never Session
```

Therefore the immediate failure is not indexing or Chinese recall. The strict
chat decision returns before hydration because development single-user requests
have no database-valid Session even though the hydration function requires one.

Product research:

- Dify Knowledge documents Knowledge as additional LLM context, binds Knowledge
  to an application, supports multi-path retrieval, semantic/keyword weighting,
  rerank, TopK, score thresholds, metadata filters, and citations;
- LangChain documents 2-step, agentic, and hybrid RAG; hybrid adds query
  enhancement, retrieval validation, and answer validation;
- the selected mm-chat direction combines reliable per-message retrieval over
  explicitly bound Knowledge with Router-controlled optional Web Search.

References reviewed:

- `https://docs.dify.ai/en/guides/knowledge-base`
- `https://docs.dify.ai/en/use-dify/knowledge/integrate-knowledge-within-application`
- `https://docs.dify.ai/en/use-dify/knowledge/test-retrieval`
- `https://dify.ai/blog/hybrid-search-rerank-rag-improvement`
- `https://docs.langchain.com/oss/python/langchain/retrieval`

Frozen decisions are recorded in
`docs/tracking/g11-knowledge-auto-rag-plan.md`. Next: implement G11.9A only,
verify the real DOCX answer and unrelated Auto fallback, record evidence, and
commit before G11.9B.

## 2026-07-17 — G11.9A Development hydration and Auto semantics

Outcome: the owner's active DOCX now reaches the selected model as optional
Knowledge context. Strict refusal is gone: a relevant query answers with
`[K1]`, while a normal miss continues through the model without an empty
Knowledge card.

Implemented flow:

- development startup creates/rotates a fixed-owner internal Postgres Session;
  its random hash has no browser token, and development middleware ignores stale
  browser Bearers while injecting the database-valid Session;
- standalone startup idempotently provisions server-owned answer governance,
  owner query consent, and Personal-collection answer consent for the configured
  server-default model;
- answer-only consent changes no longer advance
  `collection_processing_revision`, because they do not change parse,
  embedding, or published search bytes;
- any selected collection now triggers bounded retrieval; hydrated and governed
  evidence augments the ordinary streaming provider request, while no evidence
  or a Knowledge dependency failure falls back to the model;
- Auto instructions permit general model knowledge, request `[K<n>]` markers
  for Knowledge-backed claims, and never reject a useful answer solely because
  the provider omitted a marker;
- frontend stream/message payloads no longer emit `ragStrict` or
  `knowledgeStrict`; normal misses render nothing, citations render the existing
  source card, and true dependency failures render one lightweight notice.

Runtime correction discovered during deployment:

- the first answer-consent backfill advanced the collection processing revision,
  immediately making pre-existing published materializations fail the current
  projection fence;
- the code was corrected so answer-only grant/revoke/expiry events retain the
  projection revision; the two already affected local collections were repaired
  only after proving `current = published + 1`, a current answer-only consent,
  and answer grant time later than publication;
- migration `026_rag_cjk_bigram_normalization` additionally removes
  locale-dependent `[:alnum:]` gating and strips common ASCII/CJK punctuation
  before bounded bigram generation, so `研究方向是什么？` follows the same lane as
  the punctuation-free query.

Live proof:

```text
internal development session id       00000000-0000-0000-0000-000000000002
session database state                 active, unrevoked, seven-day expiry
test collection answer authority       openai_compatible/server-default/gpt-5.5
test collection projection revision    4 before restart / 4 after restart
candidate: 研究方向是什么？             1, document e7845c02-...1477
answer                                 推荐系统 + generated recommendation [K1]
persisted knowledge mode/outcome       auto / answered
persisted citation count               1
unrelated: 今天天气如何                 ordinary model answer
unrelated knowledge outcome/card       no_evidence / hidden
same-origin /mm-api config              200 local
temporary consent-revision collection  revision 4 with 3 processing + 1 answer consent
temporary collection cleanup           DELETE 204
```

Verification:

```text
Go all-package compile                         passed
Go focused chat/http/auth/knowledge/migration passed
frontend focused tests                        6 passed
frontend full tests                           854 passed; 1 sandbox-only spawnSync EPERM
frontend format/typecheck/lint                 passed / passed / passed
Docker backend/frontend source builds         passed / passed
backend/frontend health                       healthy / healthy
real gpt-5.5 Knowledge answer                 answered with [K1]
real gpt-5.5 unrelated fallback               completed, no refusal
```

The full frontend failure is environmental and unchanged:
`byokGenerateScript.test.ts` cannot `spawnSync /usr/bin/node` in the restricted
sandbox. Docker production build and the other 854 tests passed.

Rollback surface: remove development Session injection and answer bootstrap,
restore the prior handler branch, and roll back migration `026`. Do not restore
the strict refusal UX. Next slice: G11.9B conversation-persistent Knowledge
binding and the dedicated composer control.

## 2026-07-17 — G11.9B Conversation-persistent Knowledge binding

Outcome: Knowledge selection is now conversation state rather than a field
repeated on every user/stream request. Selecting once survives the server
conversation round trip; subsequent send, regenerate, and edited-message branch
requests no longer carry a competing Knowledge authority.

Frozen contract:

```text
PATCH /v1/chat/conversations/{conversationId}
config.selectedKnowledgeCollectionIds = UUID[]
maximum                                      8
empty []                                     explicit unbind
missing key                                  new/unmigrated conversation
invalid UUID / more than 8                   INVALID_RAG_SELECTION
```

Implemented flow:

- Go validates, deduplicates, and persists the canonical collection list in
  `conversations.metadata`, and loads that user-scoped conversation before RAG;
- a present canonical key wins even when empty, so stale request/message
  metadata cannot reactivate a removed collection;
- when the canonical key is missing, one non-empty legacy request or user
  message selection is normalized and written into the conversation once;
- frontend conversation DTO mapping preserves canonical IDs including explicit
  empty arrays, and the server chat store patches/replaces only the returned
  Postgres-backed session snapshot;
- server mode exposes a dedicated `Library` control beside the paperclip,
  seeds the modal from the current conversation, enforces the eight-collection
  cap, and renders removable persistent chips above the textarea;
- the local compatibility path keeps the old attachment selector, while server
  send performs only a bounded one-time migration of an old Knowledge
  attachment when the conversation has no canonical binding;
- new blank conversations have no binding. Opening Knowledge before the first
  message creates the conversation and then saves the chosen binding.

Verification:

```text
Go all-package compile                                  passed
Go focused binding/validation/legacy-migration tests   passed
frontend format / lint / typecheck                     passed / passed / passed
frontend focused tests                                 47 passed
frontend full tests                                    855 passed; 1 sandbox-only spawnSync EPERM
Docker backend/frontend production source build        passed / passed
backend/frontend recreated health                      healthy / healthy
```

The unchanged full-suite failure is
`byokGenerateScript.test.ts: spawnSync /usr/bin/node EPERM`. A same-origin curl
replay from the restricted host could not be executed because the escalation
review service returned an unsupported-review-model error; the source-built
containers themselves reached healthy state. The next owner browser smoke
should confirm chip naming/removal and refresh persistence before G11.9G final
closure.

Rollback surface: remove the dedicated control/store action and restore the
legacy stream payload only together. Backend read compatibility may remain,
but a rollback must not leave request metadata and conversation metadata as two
simultaneous long-term authorities. Next slice: G11.9C contextual hybrid
retrieval and rerank.

## 2026-07-17 — G11.9C.1 Contextual rewrite and dual-query RRF

Outcome: the first bounded G11.9C slice is complete. Independent questions keep
one retrieval request; only context-dependent follow-ups trigger a bounded model
rewrite, and retrieval searches both the untouched user question and the
standalone rewrite.

Implemented flow:

- strong English/Chinese deictic markers gate rewriting; exact identifiers and
  independent questions do not spend a rewrite call;
- rewrite input includes at most six prior user/assistant messages, excludes the
  current message, bounds each history item, preserves exact identifiers, and
  requests only one standalone query;
- rewrite failure, empty output, oversize output, or unchanged output silently
  keeps the original retrieval lane;
- the assembler fetches up to 20 reference-only candidates for each active
  query lane, deduplicates exact fenced references, and fuses ranks with
  deterministic RRF (`k=60`);
- RRF sorting is global across all selected collections and hydrates at most
  five references through the unchanged Go reauthorization boundary;
- persisted Knowledge diagnostics record only `queryRewritten=true|false`, not
  the private query or conversation content.

Verification:

```text
Go all-package compile                         passed
Go vet                                        passed
focused rewrite / dual-query / RRF tests       passed
handler end-to-end contextual follow-up test   passed
Docker backend production source build         passed
backend/frontend health                        healthy / healthy
```

This slice deliberately does not claim Dense or rerank completion. The current
keyword/CJK database function remains the only candidate lane until G11.9C.2
adds the private Python Jina query-embedding path. Rollback: omit the rewritten
query from `RAGAssemblyInput` and restore the one-lane fetch; hydration and
authorization contracts are unchanged.

## 2026-07-17 — G11.9C.1 Live model-governance regression closure

Outcome: switching the active administrator-configured model from the
environment fallback `gpt-5.5` to `gpt-5.6-sol` no longer degrades a valid
Knowledge hit to `answer_governance_required`.

Root cause:

- conversation binding, candidate recall, and hydration were healthy;
- the assistant row used `openai_compatible/gpt-5.6-sol`, but startup had
  provisioned Answer governance and consent only for `PROVIDER_MODEL=gpt-5.5`;
- the exact model fence therefore rejected otherwise valid evidence and fell
  back to an ordinary model answer with the dependency notice.

Correction:

- development startup merges the environment model list with every enabled
  Postgres `provider_configs` model, normalizes the Server Default processor to
  the runtime `openai_compatible` identity, trims, deduplicates, and sorts the
  resulting authorities;
- custom server-stored providers retain their exact provider ID as processor;
- every identity receives governance, owner query consent, and backfill for all
  existing Personal collections; new collections receive every configured
  Answer consent through the same automatic provisioning list;
- disabled providers and encrypted secret references are ignored by identity
  derivation, and provider-config read failure keeps startup fail-closed.

Live proof against the active `test` collection:

```text
stored Server Default models        gpt-5.6-sol/terra/luna, gpt-5.5, gpt-image-2
gpt-5.6-sol query consent            granted
gpt-5.6-sol test collection consent granted
question                            研究方向是什么？
real answer                          推荐系统 + generated recommendation [K1]
knowledge outcome                    answered
citation count                       1
answer authority                     openai_compatible/server-default/gpt-5.6-sol
temporary live-proof conversation    deleted; active count 0
```

Verification:

```text
Go focused cmd/api + Knowledge tests passed
Go all-package vet                   passed
Go full tests                        all non-network packages passed;
                                     existing sandbox httptest bind denied
Docker backend production build      passed
backend/frontend/RAG health          healthy / healthy / healthy
real provider Knowledge stream       HTTP 200 + message.completed + [K1]
```

Rollback: restore the single environment identity only if the administrator UI
is also prevented from selecting other models. Rolling back one without the
other recreates the exact live failure.

## 2026-07-17 — G11.9C.2 Jina Dense retrieval and keyword degradation

Outcome: the bounded Dense slice is complete without claiming rerank. Go now
requests a private 1024-dimensional Jina `retrieval.query` vector, Postgres
fuses selected-collection keyword/CJK and Dense reference lanes with RRF, and a
Jina/query-gateway failure falls back to the existing keyword function.

Implemented boundaries:

- Python exposes only `POST /internal/retrieval/query-embedding`, protected by
  the existing internal Bearer token; request/query/response sizes, exact JSON
  shape, model, dimensions, finite values, and non-zero norm are bounded;
- Jina passage indexing remains `retrieval.passage`; query embedding uses
  `retrieval.query`, both on `jina-embeddings-v4` at 1024 dimensions;
- Go uses a redirect-disabled, bounded private client and never logs the query,
  internal token, provider response, or API key;
- migration `027` preserves selected collection, active generation/head,
  published materialization, visibility/revision, deletion, active version,
  ready-vector, reference-only, and later Go reauthorization fences;
- SQL cosine is computed over the existing `REAL[1024]` projection, then
  keyword and Dense ranks are fused with deterministic RRF (`k=60`);
- live calibration proved one absolute threshold was insufficient: the short
  weather query scored `0.553727`, above relevant paraphrases. The conservative
  pre-rerank gate therefore requires at least eight query characters and
  cosine `>= 0.48`. C.3 rerank remains responsible for broader relevance.

Verification:

```text
Python Ruff / strict Mypy                    passed / passed
Python focused tests                         20 passed
Go focused tests / vet                       passed / passed
temporary Postgres migrations                001 -> 027 passed
027 runtime role probe                       go_api_runtime execute passed
Postgres Dense + short-negative integration  2 passed
private real Jina endpoint                   v4 / 1024 / finite non-zero
no-lexical-overlap semantic probe             keyword=0 / hybrid=1 / 0.499906
short weather / long weather / cooking        hybrid=0 / 0 / 0
real gpt-5.6-sol Knowledge stream             completed / answered / [K1]
Jina stopped + keyword question               completed / answered / [K1]
backend/frontend/RAG/Postgres/Redis/MinIO      healthy
temporary conversations/sessions/database     deleted
```

The real negative calibration also showed why C.2 must not claim production
relevance ranking: a Dense embedding score is not a calibrated relevance
probability. G11.9C.3 remains open for Jina rerank, evaluated thresholds,
cross-collection TopK, and rerank-only failure proof.

Rollback: disable the query client by leaving the internal token blank or
remove the query-gateway URL to retain keyword-only behavior. A database
rollback drops only migration `027`'s hybrid reference function; passage
vectors, source chunks, hydration ACLs, and conversation data remain intact.

## 2026-07-17 — G11.9B.1 Knowledge state visual cleanup

Outcome: the composer now communicates Knowledge binding through state rather
than permanent decoration. With no selected collection, the dedicated Library
button uses the same neutral gray palette as adjacent tools; after a collection
is bound it uses the existing purple active treatment. Merely opening the
selection modal does not mark the control active.

The citation header keeps its count and expand/collapse behavior but removes
the redundant `AUTO` mode badge. Auto retrieval remains the backend behavior;
only the unexplained visual label was removed.

Verification:

```text
focused frontend composition tests  6 passed
frontend ESLint / typecheck          passed / passed
frontend Prettier check              passed
frontend production source build     passed
frontend/backend health              healthy / healthy
```

Rollback: restore only the removed badge and prior button classes; no
conversation binding, retrieval, or citation metadata contract changed.

## 2026-07-17 — G11.9B.2 Citation helper-copy cleanup

Outcome: expanded citation details no longer display the redundant sentence
“回答已使用经过验证的知识库证据。” The citation heading, count, source title,
snippet, locator, and collapse interaction already communicate that state.

The unused `verifiedEvidenceUsed` key was removed from all three locale files.
The standalone frontend README now records the durable UI-copy rule: visible
helper text must enable an action, explain an error, or resolve real ambiguity;
it must not merely repeat state already expressed by title, icon, color, count,
or content.

Verification:

```text
focused citation/composition tests  9 passed
frontend ESLint / typecheck         passed / passed
frontend Prettier check             passed
```

Rollback: restore the paragraph and all locale keys together. No retrieval,
citation metadata, or accessibility contract changed.

## 2026-07-17 — G11.9C.3 Jina rerank and global cross-collection TopK

Outcome: G11.9C is complete. Candidate references are globally fused, source
text is reauthorized and hydrated inside Go, exact query/collection rerank
consent is checked before egress, and the private Python boundary calls
`jina-reranker-v3`. Scores `>= 0.0` survive and at most five chunks across all
selected collections reach answer generation.

Implemented boundaries:

- private `POST /internal/retrieval/rerank` uses the existing internal Bearer,
  caps the query at 2048 bytes, accepts at most 20 documents/64 KiB each/1 MiB
  total, and requires one unique finite score for every input index;
- Jina request pins `top_n` to input count and disables returned documents and
  embeddings; fixed errors never echo query, source text, keys, token, URL, or
  provider response;
- Go authorizes before hydration/egress, hydrates 20 candidates in `16 + 4`
  batches, prefers a standalone rewritten query, applies the global threshold
  and Top5, and persists only `rerankStatus=applied|degraded|disabled`;
- missing governance sends no source text and retains bounded RRF order;
  provider/response failure degrades to RRF Top5, while consent/DB failure stays
  an observable dependency error; a valid all-negative rerank is a normal miss;
- development startup provisions fixed Jina rerank governance plus owner query
  and all current/future Personal collection consent.

The first source-built live replay exposed one cross-layer defect before
promotion: adding rerank consent advanced `collection_processing_revision`, but
rerank is query-time authority and does not change indexed bytes. Both active
collections became one revision newer than their published materializations,
so all candidates were correctly fenced out. `collectionConsentAffectsProjection`
now excludes both `answer` and `rerank`; parse/passage-embedding consent still
invalidates the projection. Focused unit/Postgres tests cover the rule. The two
local revisions created by the uncommitted faulty build were restored with
exact ID/current-revision guards, and a subsequent backend restart preserved
revision parity.

Real supplier/runtime proof:

```text
private endpoint model                       jina-reranker-v3
private endpoint positive/negative score     0.57441278 / -0.08286887
governance head                              active, head revision 2
owner query + both collection consents       granted, rerank/text/plain
real gpt-5.6-sol positive                    completed / answered / [K1]
positive diagnostic                          rerankStatus=applied
rerank URL isolated to 127.0.0.1:1           answered / [K1] / degraded
restored two-collection query                 applied / [K1],[K2]
citation collection count                    2
temporary active G11.9C.3 conversations       0
backend/rag-worker                            healthy / healthy
```

Verification:

```text
Python Ruff / strict Mypy                    passed / passed
Python focused rerank/health tests           30 passed
Go Knowledge/ragproviders/config tests       passed
Go focused chat RAG/rerank tests             passed
Go vet ./...                                 passed
Docker backend + RAG source builds           passed / passed
real applied/degraded/cross-collection       passed / passed / passed
```

Rollback: leave `RAG_SOURCE_GATEWAY_TOKEN` blank or remove the reranker wiring
to keep hybrid/RRF Top5. Do not roll back the query-time consent revision rule;
doing so silently invalidates otherwise current search projections. G11.9D was
not started.

## 2026-07-17 — G11.9D.1 Structure-aware chunk planning

Outcome: the first D slice is complete without touching the active generation.
Source inspection proved the database/projection contract already supports
Parent/Child rows, heading paths, locator summaries, chunk-block spans, and
overlap counters. The actual baseline defect is earlier: both Native and MinerU
currently flatten parsed content before constructing their Chunk Manifest.

Added a pure deterministic planner that:

- accepts contiguous validated heading/paragraph/list/table/code/formula units;
- emits only unit ordinals plus UTF-8-safe half-open byte ranges, so later
  projection must clip existing locators rather than invent coordinates;
- keeps Parents inside one heading path, targets 1,200–1,600 tokens, and caps
  them at 2,000;
- targets 300–500-token Children with a hard cap of 650 and exact adjacent
  source-range overlap capped at 100 tokens;
- preserves protected table/code/formula/heading units while they fit and
  bounds units, total bytes, kinds, heading paths, and deterministic replay;
- performs no filesystem/network/database/provider/clock/random operation.

Verification:

```text
focused Ruff                         passed
strict Mypy                          passed
planner unit tests                  10 passed
deterministic replay                 byte/range identical
multilingual UTF-8 boundaries        passed
table-row atomicity                  passed
RAG production source build         passed
packaged planner import/proof        parents=1 / children=1
active Index Generation mutations   zero
```

Rollback: remove the planner and contract only; no runtime caller or persisted
generation depends on them. Next slice D.2 maps validated Native/MinerU
structure and locators into the existing Canonical IR/Chunk Manifest contract,
then stages a new generation without re-upload.

## 2026-07-18 — G11.9D.2.1 Native structural artifact projection

Outcome: the Native half of structural Artifact projection is complete as an
offline proof boundary. Production `NativeSandboxParserGateway` remains on the
old text baseline because the new chunk profile is not yet bound to a staged
Search Profile/Index Generation. No parser route, materialization, provider,
or active generation was changed.

Added `mm_chat_rag.native_structure_artifacts` with a deterministic flow:

```text
source-bound NativeDocument
  -> unique structural units
  -> D.1 Parent/Child planner
  -> Canonical IR v2 + clipped Source Locator v2
  -> Chunk Manifest v2
  -> build_postgres_projection_batch DTOs
```

The mapper preserves headings, standalone paragraphs/code, list items, and
table rows without duplicating their nested Native nodes. Table cells use the
fixed `" | "` representation; external-target fragments are excluded. Heading
ancestry uses stable logical block IDs. Exact identity fragments are clipped to
verified raw/scalar/line ranges, while syntax-decoded fragments keep their
coarse verified source position rather than fabricating coordinates.

Planner ranges become block-relative chunk spans with deterministic source
hashes and clipped locators. Adjacent ranges use an empty joiner; cross-block
ranges use Chunk Manifest v2's frozen double-newline separator. Overlap is a
byte-identical previous-child fragment with explicit adjacent-child identity.
Parent section ownership uses its first structural block and Child ordinals are
globally contiguous for the current Postgres projector.

Verification:

```text
focused Native Artifact tests             4 passed
DOCX heading/paragraph/table row           passed
Markdown heading/list/table/code           passed
long multilingual Parent/Child/overlap     passed
UTF-8/CRLF/CR identity locator clipping    passed
Canonical IR + Chunk Manifest schemas      passed
Postgres projection DTO proof              passed
Ruff src/tests                             passed
strict Mypy src                            passed
non-socket regression suite                1676 passed / 7 skipped
production RAG source build                passed
packaged image import                      64 / callable=True
active Index Generation                    46a1c7bb-44ed-4868-9d61-edd557f9d3f0
active-generation mutations                zero
```

The sandboxed all-test attempt reached `1715 passed / 7 skipped`; its 16
failures were existing socket/bind/JCS child-runtime gates denied with `EPERM`,
not assertion failures in this slice. The focused suite, schema gates, static
checks, source build, and packaged import are green. A host-permission rerun is
still required for those socket-owning tests.

Rollback: delete the new builder/tests and revert these documentation entries.
No live caller or persisted row depends on this slice. Next is D.2.2 MinerU
structure/page-locator mapping; only D.2.3 may stage a new generation and spend
real Jina passage-embedding quota.

## 2026-07-18 — G11.9D.2.2 MinerU structural artifact projection

Outcome: the admitted MinerU page-element half of Artifact projection is
complete without provider, database, gateway, or generation side effects.
The mapper consumes the existing decoded and digest-bound archive mapping
input, using `middle_json.pages[].elements[]` as structure authority instead of
flattening compatibility `full.md`.

Known heading/text/list/quote/code/table/formula/footnote/header/footer elements
become Canonical blocks. Table rows and cells render with fixed separators.
Every page and text span retains the admitted page index and BBox; chunk
clipping narrows canonical bytes while keeping the source element geometry.
Unknown text-bearing kinds fail closed, while non-text images are not indexed.

Verification:

```text
focused MinerU structural tests          2 passed
combined Mapper/Gateway/planner tests     136 passed
Ruff src/tests                            passed
strict Mypy src (60 modules)              passed
non-socket regression suite               1678 passed / 7 skipped
synthetic heading/text/table/formula      passed
page-BBox Postgres projection             passed
multilingual UTF-8/exact overlap          passed
Canonical IR + Chunk Manifest schemas     passed
deterministic replay                      passed
production RAG source build               passed
packaged image import                     64 / callable=True
active Index Generation                   46a1c7bb-44ed-4868-9d61-edd557f9d3f0
active Index Generation mutations         zero
```

Rollback: remove the MinerU mapper/tests and this documentation entry. No live
caller depends on them. D.2.3 must replay a real provider archive to detect
shape drift, stage Native/MinerU projections in a new generation, spend real
Jina passage-embedding quota, and leave the current active generation untouched
until verification.

## 2026-07-18 — G11.9D.2.2a Mixed-format profile convergence

D.2.3 preflight found that Native and MinerU had distinct structure chunk
hashes, while one Index Generation binds exactly one
`knowledge_index_profiles.chunk_profile_hash`. Real mixed PDF/DOCX staging
would therefore fail closed. Both mappers now alias the shared D.1
`STRUCTURE_CHUNK_PROFILE_HASH`; mapper/artifact identities remain distinct.

Ruff, strict Mypy, and 136 combined planner/Native/MinerU/Gateway tests passed;
the active generation remained `46a1c7bb-44ed-4868-9d61-edd557f9d3f0`.

Rollback is the commit revert only. No live profile or generation was created.

## 2026-07-18 — G11.9D.2.3a Candidate rebuild allocator

Added migration `028_structure_generation_rebuild_allocator` with the
`knowledge_begin_structure_generation_rebuild(...)` mutation boundary. It
locks the corpus head, refuses concurrent building/verified candidates,
requires the allocation JSON to exactly cover every current active/available
document once, clones active provider settings into shared structure
Index/Search Profiles, and creates only a non-active building generation,
projection state, staging materializations, and pending `parse/reprocess` jobs.

The first disposable-clone proof exposed three integration defects before any
production migration: missing Index Profile grants for the function owner, an
incorrect collection processing-revision column, and row-lock permissions on
tables the projection owner intentionally cannot update. After those narrow
fixes, review found a fourth boundary defect: count equality alone allowed a
same-cardinality substituted document set. The allocator now proves exact set
membership as well as count and uniqueness.

Live proof used a disposable clone of the current three-document corpus. Both
a two-document allocation and a three-entry set with one substituted UUID
failed with `RAG_STRUCTURE_REBUILD_ALLOCATION_COVERAGE_INVALID`. The valid call
created generation `33333333-3333-4333-8333-333333333328` as sequence 4 with
three staging materializations and three pending parse jobs. A second call
failed with `RAG_STRUCTURE_REBUILD_CANDIDATE_EXISTS`; the active generation
remained `46a1c7bb-44ed-4868-9d61-edd557f9d3f0`. The disposable database was
then dropped and absence verified.

Go migration tests, full backend tests, `go vet ./...`, and Compose source
builds for backend/migrate passed. Migration 028 was not applied to the
production database. Rollback drops the function and its added profile grant.
Remaining D.2.3 work owns real MinerU archive replay, candidate parse
projection, real Jina passage embeddings, verification, and eventual D.3
cutover.

## 2026-07-18 — G11.9D.2.3b Candidate structure parse projection

Outcome: the candidate generation can now consume its allocated parse jobs
without changing baseline-generation behavior. Migration 029 adds
`knowledge_resolve_parse_chunk_profile(...)`, which resolves the generation's
Index Profile only through the current worker/lease-token/job/materialization
fence. The Python authority router keeps the old Native/MinerU baseline mappers
for the baseline hash and selects the shared structure mappers only for the
shared structure hash. Unknown profiles and processors fail closed.

The first real PDF run exposed provider shape drift rather than a projection
error. The downloaded MinerU archive used `layout.json.pdf_info[]`, containing
`para_blocks`, `discarded_blocks`, `page_size`, and line/span content, while the
frozen synthetic fixture used `pages[].elements[]`. The mapper now admits both
closed shapes, orders the live blocks by BBox/index, joins span content, scales
point geometry to milli-points, and still rejects unknown text-bearing shape.
The saved real archive passed Canonical IR, Chunk Manifest, and Postgres DTO
validation with four page-BBox blocks.

Live replay found two additional boundary defects. First, the old replay
function evaluated `clock_timestamp()` independently for `available_at` and
`created_at`, so the earlier field could precede the later field by
microseconds and violate `knowledge_processing_jobs_available_after_created`.
Migration 030 now uses one timestamp for successor and audit fields. Second,
default `httpx` INFO logging could include a signed result URL. Inline URL query
redaction plus WARNING thresholds for `httpx`/`httpcore` now prevent that leak;
the final worker log scan found zero raw query URLs.

The disposable-clone proof used a parse-only worker, the two existing Native
DOCX sources, and one real MinerU PDF result. Docker/WSL could not fetch the
provider CDN directly in this environment, so a temporary exact-host/path
Windows `curl.exe` proxy was used only for the result ZIP and removed afterward.
No Jina passage-embedding stage was enabled.

Verification:

```text
real MinerU archive mapping                 4 blocks / 1 parent / 1 child
latest candidate parse jobs                1 PDF + 2 DOCX / all succeeded
candidate materializations                 3 staging
shared structure Child profile             all matched
PDF locator kind                           page_bbox x4
passage-embedding jobs                     3 pending / 0 consumed
active generation                          unchanged
replay available_at = created_at           true
raw signed-query URLs in final worker log  0
focused structure/Postgres/log tests       53 passed
Ruff check                                 passed
changed-file Ruff format                   passed
strict Mypy                                70 modules passed
RAG full tests                             1740 passed / 7 skipped
Go migration + full backend tests          passed
Go vet                                     passed
backend/migrate/rag-worker source builds   passed
```

The repository-wide RAG format check still reports the pre-existing clean file
`src/mm_chat_rag/health.py`; every file changed by this slice passes formatting.

Cleanup removed both `mm-chat-d23b-*` containers, the disposable database,
the Windows result proxy and isolated Chrome state, downloaded archives, logs,
and the temporary credential-bearing environment snapshot. Final proof showed
zero temporary containers/databases, formal migration max `27`, no formal
allocator function, and the expected formal active generation unchanged.

Rollback: revert migrations 029/030, the profile-aware router/structure gateway
composition, real-shape mapper fallback, redaction hardening, tests, and these
documents. Migration 030 down restores the previous replay definition; do not
roll it back after relying on replay timestamps without first checking pending
successors. Production has not applied migrations 028–030, so this commit does
not require a live database rollback.

Remaining work: run real Jina passage embeddings for the three staged Children,
verify candidate completeness/hashes/deletion fences, then enter D.3 for atomic
generation cutover and live citation proof.

## 2026-07-18 — G11.9D.2.3c Real Jina candidate embedding closure

Outcome: D.2.3 is complete without introducing another provider or persistence
path. The existing promoted passage-embedding handler consumed the shared
candidate's three pending jobs through real Jina `retrieval.passage`, validated
1024-dimensional finite vectors and float32 hashes, staged them through the
token-fenced Postgres gateway, proved each materialization search-complete, and
published each materialization/document projection head. The candidate stayed
`building`; no generation verification or promotion function was called.

The first clone attempt produced a false permission failure because the clone
command used `pg_dump --no-privileges`, stripping the runtime role ACLs that are
part of the production schema contract. Recreating the clone with ACLs intact
made the unchanged allocator pass. Future live integration clones must preserve
grants; testing privileged functions on an ACL-stripped dump is invalid
evidence.

The final no-mock run cloned the current three-document corpus, applied only
migrations 028–030, allocated the candidate, and ran one worker with exactly
`parse,passage_embedding`. A temporary exact-host/path Windows `curl.exe`
result proxy handled the known Docker/WSL MinerU CDN transport boundary. The
worker claimed and succeeded six jobs total. Jina and MinerU credentials came
only from the temporary environment snapshot and were never printed or added
to Git.

Verification:

```text
candidate document coverage                 exact 3 / 3
parse jobs                                  3 succeeded / attempt 1
real passage-embedding jobs                 3 succeeded / attempt 1
candidate materializations                  3 published
materialization manifest/result hashes      3 complete
shared-profile Children                     3
ready Jina vectors                          3 x 1024
candidate document projection heads         3 published
candidate generation/readiness              building / building
candidate actual Parents / Children         3 / 3
formal active generation                    unchanged
worker job metrics                          6 claimed / 6 succeeded
raw query URLs / credential patterns logs   0 / 0
focused Ruff                                passed
strict Mypy                                 70 modules passed
focused embedding tests                     86 passed / 1 skipped
Go migration tests                          passed
```

Cleanup removed the temporary worker/backend containers, disposable database,
Windows proxy process/files, logs, and both credential-bearing environment
snapshots. Final checks showed zero D.2.3c containers/database/files, formal
migration max `27`, and the expected formal active generation unchanged.

No production code changed in this slice: the goal was to prove the already
implemented Jina/fenced-publish path against the new structure candidate. The
contract, plan, design, and process records changed because live evidence now
closes D.2.3.

Rollback is documentation-only. The disposable state no longer exists and the
formal database never received migrations 028–030. G11.9D.3 now owns
generation-wide Parent/Child counters, deterministic manifest hash,
building-to-verified transition, deletion-fence/failure proof, atomic cutover,
and live citations.

## 2026-07-18 — G11.9D.3a Generation completeness verifier

Outcome: migration 031 adds the generation-wide verification boundary without
exposing promotion. `knowledge_verify_structure_generation(...)` locks the
expected corpus head, rejects the active generation, and derives exact current
document coverage, latest Parse/Embedding success, published candidate heads,
parser artifacts, Blocks, Parent/Child containment, shared profile, locators,
and ready Jina 1024 vectors entirely from persisted evidence.

The verifier now promotes parser artifact sets from staging to verified inside
the same transaction, computes ordered versioned row digests for
materializations/artifacts, Blocks, Parents, and Children/search vectors, then
freezes one generation manifest and document/Parent/Child counts. It updates
only candidate generation `building -> verified` and projection state
`building -> ready`. Calling it again recomputes all evidence and must return
the identical manifest/counts; it contains no call or grant to the promotion
function and no active-head update.

The first live call exposed one SQL naming defect: the local `verified_at`
variable collided with the parser-artifact column inside `UPDATE`. Renaming the
single transaction timestamp to `verification_time` removed the ambiguity. The
migration was rolled down and rebuilt before the successful proof, so the final
evidence exercised the packaged migration rather than an ad hoc database edit.

Live proof rebuilt the real three-document D.2.3 candidate on an ACL-preserving
clone and completed six credential-backed Parse/Jina jobs. Verification returned
3 documents, 10 Blocks, 3 Parents, and 3 Children. Immediate verified replay
returned the same manifest and every count. A negative transaction removed one
ready vector; the verifier rejected it with
`RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE`. Connection rollback restored all
three vectors, three verified parser artifact sets, and the bound
`verified/ready` manifest.

Verification:

```text
pending candidate before D.2.3             coverage rejected
real Parse / Jina jobs                     3 + 3 succeeded
verified document / Block counts           3 / 10
verified Parent / Child counts             3 / 3
generation / projection state              verified / ready
generation/state manifest equality         true
deterministic verified replay              hash + all counts stable
missing ready-vector negative              rejected / rollback restored
verified parser artifact sets              3
formal active generation/head revision     unchanged
Go migration tests                         passed
Go full tests                              passed
Go vet                                     passed
backend/migrate source build               passed
```

Cleanup removed the worker/backend containers, disposable database, Windows
result proxy, provider logs, and credential-bearing environment snapshots.
Final proof showed zero G11.9D.3a temporary resources, formal migration max
`27`, verifier absent from formal, and the expected formal active generation
unchanged.

Rollback: migration 031 drops the verifier and revokes its narrow parser-set
status/verified-time update grant. Production has not applied migrations
028–031. If rollback is ever required after verification in another database,
first account for already-verified candidate/artifact rows; the down migration
removes code/privilege but deliberately does not rewrite historical state.

Remaining work: G11.9D.3b proves deletion/race and failed-candidate rollback;
G11.9D.3c performs the separately fenced atomic cutover and live citation proof.

## 2026-07-18 — G11.9D.3b Deletion/concurrency fence and failed rollback

Outcome: migration 032 closes the stale-verified-candidate gap without
performing a successful cutover. The prior promotion function trusted
`verified/ready` plus the frozen manifest; deleting a document after
verification could therefore leave a candidate that no longer represented the
current corpus. Promotion now locks the expected corpus head, resolves the
candidate's persisted chunk-profile hash, and reruns
`knowledge_verify_structure_generation(...)` in the same transaction before
any active-generation mutation.

The existing `DeleteDocument` transaction already reaches
`resolvePurgeProjectionBinding`, which locks the same corpus-head row before the
document/version tombstones commit. No new deletion path was added. Promotion
and deletion now serialize on one database authority, and the verifier checks
the post-lock current corpus rather than trusting pre-delete evidence.

Migration 032 also adds `knowledge_fail_structure_generation(...)`. It requires
the expected head revision, exact candidate manifest, and bounded failure code;
locks the candidate generation and projection state; and atomically changes
only `verified/ready -> failed/failed`. An identical call is an idempotent
success, while a mismatched replay fails closed. The active generation and head
are never updated. Since the existing candidate uniqueness index covers only
`building|verified`, the failed row immediately releases the rebuild slot.

The ACL-preserving disposable clone first rebuilt the real three-document
PDF/DOCX corpus, completed all Parse and real Jina passage-embedding work, and
verified the candidate. Tombstoning the PDF caused hardened promotion to fail
with `RAG_STRUCTURE_VERIFY_COVERAGE_INVALID`; failure rollback and immediate
replay succeeded, and a replacement candidate was allocated for the remaining
two DOCX documents. That replacement again completed two Parse plus two real
Jina jobs and verified successfully.

The decisive concurrency proof opened deletion first, locked the corpus head,
slept two seconds, then tombstoned one DOCX. Promotion began 200 ms later and
waited 1,908 ms behind that lock. After deletion committed, its in-transaction
verifier rejected stale coverage with
`RAG_STRUCTURE_VERIFY_COVERAGE_INVALID`. Active generation
`46a1c7bb-44ed-4868-9d61-edd557f9d3f0` and head revision `4` were unchanged.
The second candidate then failed atomically, identical replay returned true,
and a third one-document `building/building` allocation proved the slot was
free again.

Verification:

```text
delete-before-promotion stale candidate       coverage rejected
concurrent delete/promotion serialization     1,908 ms lock wait
post-delete promotion                         coverage rejected
first/second candidate failure state          failed / failed
identical fail replay                          true / true
replacement allocation after each failure     succeeded
successful active cutover                      not executed / not granted
active generation / head revision              unchanged / 4
migration 032 down/up replay                    passed
Go migration + full tests                      passed
Go vet                                         passed
backend/migrate source build                   passed
```

Cleanup removed both D.3b containers, the disposable database, Windows result
proxy/files, SQL outputs, and credential-bearing environment snapshots. Final
checks showed zero D.3b temporary resources; the formal database remained at
migration 27, neither verifier nor failure function existed there, the formal
promotion body remained pre-fence, and the active generation/head were
unchanged.

Rollback: migration 032 revokes and drops the failure function and restores
migration 010's promotion definition. It intentionally does not rewrite
persisted failed candidates. Production has not applied migrations 028–032, so
this commit requires no live database rollback.

Remaining work: G11.9D.3c is the sole successful-promotion gate. It must grant
the narrow cutover caller, rebuild and verify a current candidate, atomically
switch the head, prove live Parent/Child citations, and exercise the defined
old-generation rollback behavior.

## 2026-07-18 — G11.9D.3c Atomic cutover, live citations, and rollback

Outcome: G11.9D is complete on a disposable production-shape clone. Migration
033 grants `go_api_runtime` the D.3b-hardened promotion function rather than
introducing a second cutover implementation. It also adds
`knowledge_rollback_index_generation(...)`, a one-step recovery function bound
to the active structure rebuild's exact source generation, both manifests, the
expected head revision, current document bytes, and target Parent/Child/ready
Jina projection completeness.

The first rollback implementation was too strict: it required target
materialization ACL/visibility/processing revisions to equal current collection
revisions. The previous active generation legitimately retained a PDF
materialization at processing revision 4 after its collection advanced to 5;
the normal query fence already hid that row. Treating visibility as rollback
coverage made the exact pre-cutover state impossible to restore. The corrected
contract requires every current document/version/file/content tuple to have its
exact target head and published materialization, then requires a complete
Parent/Child/ready-vector path. Historical or revision-stale rows may remain,
but existing query authority continues to hide them without data leakage.

The final ACL-preserving clone applied migrations 028–033 and allocated a
three-document candidate from the formal active generation. The temporary
worker used the existing Native/MinerU structure routes and real Jina
`retrieval.passage`; all three Parse and all three passage-embedding jobs
succeeded on attempt one. Three materializations published with three Parents,
three Children, and three 1024-dimensional ready vectors. The D.3a verifier
froze 3 documents, 10 Blocks, 3 Parents, and 3 Children.

Promotion executed as `go_api_runtime` and returned true. The structure
generation became `active/ready`, the old generation became
`retired/retired`, head revision advanced `4 -> 5`, and corpus projection
revision reached 12. Active-head keyword retrieval returned the structure
generation and its exact Parent/Child IDs.

The live application proof created a temporary development conversation bound
to the selected collection. The first manual stream used the persisted registry
ID `SERVER_DEFAULT`, but the non-BYOK configured provider contract expects its
normalized processor identity `openai_compatible`; retrying with that existing
identity completed. Real `gpt-5.6-sol` returned `answered`, rerank `applied`,
`[K1]`, and one citation whose generation, Parent, and Child all matched the
newly active structure projection.

Rollback failure and success were both exercised. Transactionally changing the
old generation's current ready vector to staging caused
`RAG_GENERATION_ROLLBACK_PROJECTION_INCOMPLETE`; the failed connection
transaction restored that vector and left the structure generation active.
The valid call then returned true, changed the structure generation to
`retired/retired`, restored the exact source to `active/ready`, and advanced
head revision `5 -> 6` plus corpus revision `12 -> 13`. Direct retrieval and a
second real `answered/[K1]` stream both cited the restored generation. Replaying
the original rollback inputs failed with `RAG_GENERATION_ROLLBACK_HEAD_STALE`.

Verification:

```text
real Native/MinerU Parse jobs                  3 succeeded / attempt 1
real Jina passage-embedding jobs              3 succeeded / attempt 1
verified document / Block / Parent / Child    3 / 10 / 3 / 3
promotion ACL / execution                     go_api_runtime / true
post-cutover state                             new active, old retired, head 5
new-generation direct retrieval               exact generation/Parent/Child
new-generation real model stream              answered / applied / [K1]
new-generation citation binding               generation + Parent + Child
missing old ready-vector rollback negative    rejected / transaction restored
valid source-generation rollback              true / head 6
restored-generation retrieval + model stream  old generation / answered / [K1]
stale rollback replay                          head-stale rejection
worker metrics                                 6 claimed / 6 succeeded
migration 033 down/up ACL/state proof          passed
Go migration + full tests / vet                passed / passed
backend/migrate source build                   passed
```

The Windows proxy accepted only the fixed MinerU result host/path and returned
bounded identity-encoded ZIP bytes; logs contained no credential pattern or
signed result URL. Cleanup removed the temporary backend/worker containers,
clone, Windows proxy/script, SQL/SSE outputs, and both credential-bearing env
snapshots. The formal database remained at migration 27, with migration 033
functions absent and active generation/head unchanged.

Rollback: if the structure generation is active, call the guarded rollback
before applying migration 033 down. The down migration only revokes promotion
and rollback permissions and drops the rollback function; it cannot infer or
rewrite the desired persisted active generation. Production never received
migrations 028–033 in this slice.

Remaining work moves to G11.9E: Go Web Search provider parity and SearXNG
removal. G11.9F/G11.9G still own encrypted administrator provider settings and
final Knowledge/Web/model fusion plus clean-copy closure.

## 2026-07-18 — G11.9E.1 Closed Go external-search providers

Outcome: the first Web Search slice is complete without exposing a route,
reading a Key, or consuming provider quota. New package
`backend/internal/websearch` ports the admitted Tavily, Firecrawl, Exa, and
Bocha request/response shapes from the temporary Next implementation into one
closed Go `Provider` interface. SearXNG is intentionally absent.

All four adapters share a production-only hardened client: HTTPS base URLs,
no userinfo/query/fragment/localhost/private literal, no environment proxy, no
redirect, TLS 1.2+, fixed timeouts, and DNS resolution that rejects the whole
host if any address is non-public before dialing a checked address. Provider
responses are identity JSON capped at 5 MiB; query, Key, result counts, source
content/title/URL, and image fields are separately bounded.

Provider errors expose only provider ID, a stable code, and optional HTTP
status. They never reflect the API Key, response body, query, or raw transport
error. Normalization preserves provider order while stripping fragments,
dropping invalid/private literal URLs and empty content, deduplicating URLs,
truncating fields safely at UTF-8 boundaries, and enforcing one caller cap.
There is no fallback or multi-provider fan-out.

Fixture tests cover all four endpoint/auth/payload shapes, legacy content/image
fallbacks, Tavily string/object images, Firecrawl Key omission, Exa nested
request fields, Bocha image-title correlation, unsafe endpoints, request bounds,
private result removal, status redaction, transport redaction, and the 5 MiB
response ceiling. Focused coverage is 82.7% and focused vet passes.

Verification:

```text
focused websearch tests / coverage          passed / 82.7%
Go full tests / full vet                    passed / passed
backend source build                        passed
module completeness / security / quality    passed / passed / passed
provider network calls / Key reads          0 / 0
```

This slice deliberately leaves the old Next route/UI untouched: callers cannot
reach the Go package yet, so rollback is file deletion and no persisted state is
at risk. G11.9E.2 owns the Go execution/service and model-built-in stream
boundary. G11.9E.3 owns `[W]` citations, frontend cutover, SearXNG/Next route
deletion, and the authorized real-provider smoke.

## 2026-07-18 — G11.9E.2 Go Search execution and OpenAI built-in boundary

Outcome: the Go execution boundary is complete without moving Search secrets
or frontend authority prematurely. `backend/internal/websearch` now has a
server-owned `Resolver`, validated `ActiveExecution` union, `Service`, and
authenticated `POST /v1/search` handler. A request contains only query, scope,
and result limit; it cannot select a provider, base URL, Key, or secret
envelope. The resolver returns exactly one external adapter or one admitted
model-built-in execution. Mixed/empty selections fail closed, resolver details
are redacted, and there is still no automatic fallback or fan-out.

The external route re-normalizes provider output and returns `no-store` JSON.
Selecting model-built-in execution on that route returns
`MODEL_BUILTIN_SEARCH_REQUIRES_CHAT`, because the provider tool must run inside
model generation. The normal API binary intentionally has no resolver before
G11.9F, so the registered route currently returns `SEARCH_NOT_CONFIGURED`
instead of introducing a second `.env` or browser-owned secret authority.

Runtime provider truth was checked before implementing built-in Search. Go had
only one OpenAI-compatible Chat Completions implementation and no Gemini chat
provider. The new `OpenAIProvider` therefore preserves ordinary Chat
Completions but adds streaming `/responses` requests with
`web_search_preview` only when the configured runtime type is explicitly
`OpenAI`. `OpenAI Compatible` does not claim this capability. If the active
selection requests built-in OpenAI Search against a non-capable provider, chat
returns `MODEL_BUILTIN_SEARCH_UNSUPPORTED` before creating an assistant
message; it does not fall back externally.

OpenAI Responses text, URL citations, web-search results/action sources, and
usage are parsed through redacted bounded SSE frames. Built-in sources share
the external URL/content/dedupe/UTF-8/result normalizer and emit transient
`search.results` events. This slice deliberately does not create `[W]` markers,
output blocks, or persisted citations; E.3 owns those contracts and frontend
consumption.

Verification:

```text
focused race (websearch/chat/httpserver/cmd-api)  passed
focused websearch coverage                       83.4%
Go full tests / full vet                         passed / passed
backend API source build                         passed; temp binary removed
authenticated route matrix                       /v1/search protected
external resolver/provider calls                 exactly 1 / exactly 1
request-owned Key/baseURL/provider                rejected by strict JSON body
OpenAI Responses fixture                          payload/text/sources/usage passed
built-in source dedupe/cap/URL normalization      passed
OpenAI vs OpenAI Compatible capability            admitted / rejected
upstream status/frame/resolver redaction           passed
module / security / quality / change gates         passed / passed / passed / passed
provider network calls / Key reads                 0 / 0
```

No database migration, persisted Search configuration, external request,
provider quota, frontend file, SearXNG file, or legacy Next route changed.
Rollback removes the Go route/service wiring and `OpenAIProvider` capability;
ordinary Chat Completions and the untouched legacy frontend path remain the
fallback. G11.9E.3 next cuts the frontend to Go, mints and persists `[W]`
citations, removes SearXNG and the old Next route, and performs the authorized
real-provider smoke.

## 2026-07-18 — G11.9E.3 Go Search frontend cutover and `[W]` persistence

Outcome: G11.9E is complete. A chat request with `useSearch` now resolves one
server-owned `ActiveExecution` exactly once. External execution calls
`Service.Execute` on that resolved union, bounds the direct question to 2,048
bytes, caps the configured result count at 1–10, and injects a total-bounded Web
evidence section with `[W<n>]` markers after any Knowledge context. Built-in
OpenAI source events are accumulated and deduplicated; source annotations are
known-used records, so any missing terminal markers are appended before the
message completes. No provider fan-out or fallback was added.

Every terminal path persists a bounded `type: "search"` output block and
redacted `metadata.web` records. Source cards contain stable URL-derived
citation IDs and markers; provider Keys, upstream bodies, and resolver details
are absent. A focused live Postgres test finalized, reloaded, duplicated, and
reloaded an assistant with `answer [W1]` plus its Search JSONB block. The
isolated `mm_chat_g119e3_test` database and temporary builder image were deleted
after proof.

The server-mode frontend now types and dispatches `search.results`, updates the
assistant draft without browser persistence, uses the terminal server message
as authority, restores Search sources from `outputBlocks`, and linkifies both
legacy `[1]` and Web `[W1]` while leaving `[K1]` untouched. The Search toggle is
allowed in server mode and persists on the conversation; result count is sent
as `searchResultsLimit`. Availability comes only from Go `/v1/config`. Because
the persisted Search block is intentionally separate from message text, the
renderer also backfills `message.content` only when no text block exists; this
keeps both streaming and reload answers visible without duplicating native text
blocks.

Deleted production/browser authority:

- legacy Next `/api/search` route;
- browser external adapters, service, outbound policy, client decision
  preflight, and built-in Search request flags;
- browser Search Provider/Key/Base URL settings, encrypted Search-secret
  contexts, legacy default Search env variables, and the retired self-hosted
  provider type/UI/tests;
- obsolete route/service inventories and client decision tests.

Verification:

```text
backend go test ./...                                      passed
focused race (websearch/chat/httpserver/cmd-api)           passed
go vet ./...                                               passed
frontend lint / typecheck                                  passed / passed
frontend Vitest                                            177 files / 846 tests passed
frontend production build                                  passed
built Next route inventory                                 /api/search absent
live Postgres Search output-block completion/reload        passed
isolated test database / temporary test image              removed / removed
Docker Compose backend/frontend build                      passed
runtime backend / frontend health                          healthy / healthy
legacy /api/search runtime probe                           404
Go /v1/search unauthenticated boundary                     401 UNAUTHENTICATED
real Firecrawl credentialless negative smoke               redacted 4xx ProviderError
configured gateway model list                              200; no Search model advertised
configured gateway Responses Web Search capability probe   400 upstream_error
cross-provider fallback                                    none
```

The real calls were owner-authorized. No admitted external Search Key exists in
the current environment, so G11.9E.3 deliberately proves real endpoint/error
and incompatible-gateway behavior rather than inventing a successful provider.
G11.9F owns encrypted administrator settings and the positive credentialed
activation test.

During Compose verification, recreating the old persisted Postgres container
surfaced pre-existing principal drift: the volume was initialized with
`neo_chat`, while current Compose expects `neo_chat_api`. The backend failed
SASL authentication. An idempotent least-privilege `neo_chat_api` LOGIN was
created with the Compose-local credential and granted only `go_api_runtime`;
backend and frontend then returned healthy. No migration ran and live schema
version remained 27. The missing dedicated migrator principal/ownership for
this old volume remains an explicit promotion prerequisite before any future
live migration; it was not hidden by granting migration power to the API.

Rollback is one commit plus runtime image rollback. No schema was added for Web
artifacts; they use existing message JSONB. Older code can ignore unknown Search
blocks. Next is G11.9F administrator provider secrets and positive connection
tests.

## 2026-07-18 — G11.9F.1 Provider secret vault foundation

Outcome: the first F slice is complete without touching live provider state.
The new Go `internal/providersecrets` package strictly loads one bounded
versioned keyring document intended for a read-only Docker Secret. It accepts
exactly 32-byte unpadded-base64url keys, one declared active key, and a bounded
set of retained previous keys. It exports no raw key accessor.

Secrets use fresh-nonce AES-256-GCM envelopes. Version, algorithm, key ID, and
the exact provider record context are authenticated as AAD, so ciphertext
cannot be copied between model, Search, MinerU, Jina, or future Voice records.
Envelope and keyring parsing are bounded and closed; errors contain no path,
key ID, nonce, ciphertext, plaintext, or underlying crypto detail. Rotation
decrypts with a retained key and re-encrypts with the active key;
already-current envelopes are stable no-ops.

This deliberately separates browser ingress encryption from at-rest
encryption: existing BYOK RSA envelopes protect browser-to-Go transport, while
the future Docker Secret keyring owns restart-stable database ciphertext. F.1
does not yet mount the Secret, decrypt a current admin envelope, write
Postgres, register a route, alter Compose, change current `.env`, read a real
Key, or call a provider.

Verification:

```text
providersecrets unit tests / coverage             passed / 81.7%
providersecrets race / vet                        passed / passed
backend go test ./... / go vet ./...              passed / passed
focused race (vault/runtimeconfig/httpserver/api) passed
module completeness / quality / security         passed / passed / passed
database / Compose / route / runtime mutation     none
provider network calls / real Key reads           0 / 0
```

Rollback deletes the unused package and its contract; no persisted data or
runtime configuration refers to it. G11.9F.2 next adds stable keyring config,
repository envelope compatibility, transactional import/rotation, restart
proof, and model-provider fallback removal before any production cutover.

## 2026-07-18 — G11.9F.2.1 Model-provider vault write cutover

Outcome: new administrator model-provider secrets are now restart-stable vault
ciphertext in Postgres. The administrator page still sends the existing RSA
BYOK ingress envelope. Go decrypts it only at ingress, immediately encrypts the
bytes with the Docker-Secret vault and record context
`provider:model:<userId>:<providerId>`, clears the temporary byte slice, and
persists only the bounded `A256GCM` envelope. Reads accept this envelope and the
legacy `RSA-OAEP-256+A256GCM` form during the migration window; corrupt,
unknown, copied-to-another-context, or missing-vault state fails closed through
redacted errors.

An administrator metadata save lazily imports either a legacy BYOK row or the
Server Default `PROVIDER_API_KEY` fallback. A new custom provider starts empty
and cannot inherit that fallback. Clear and replacement operations remain
available even when an old secret is unusable. No schema migration was needed:
`provider_configs.encrypted_secret_ref` remains the opaque storage column.

Compose now mounts the gitignored mode-`600` keyring only into `backend` and
`admin`. Live restart testing caught an ownership trap that static Compose
rendering could not: file-backed Compose Secrets are read-only bind mounts, so
the host UID `1000`/mode `600` source was unreadable by the image's UID `100`.
The first rebuilt backend therefore failed closed with
`provider_secret_keyring_failed`. The deployment now requires
`MM_CHAT_RUNTIME_UID/GID` to match the invoking keyring owner and runs only
those two consumers as that non-root identity. Preflight validates numeric,
non-root, owner-matching IDs plus the keyring's owner, mode, size, strict JSON,
canonical key encoding, active key, and duplicate rejection. Backend was then
healthy before and after an explicit restart with the Secret still read-only.

Live persistence proof used only the isolated `mm_chat_g119f21_test` database.
It wrote a BYOK ingress request through the real Postgres repository, verified
the database contained only an `A256GCM` envelope and no plaintext/legacy
algorithm, reloaded a fresh Vault from the same keyring to simulate restart,
and resolved the original secret. The test DSN now refuses any database name
outside `mm_chat_*_test`. The isolated database was force-disconnected and
deleted; the formal database migration fingerprint stayed unchanged at 27
rows / version 27.

Verification:

```text
focused config/vault/runtimeconfig/httpserver/api tests        passed
Postgres vault write + fresh-vault reload                       passed
isolated test database / formal database mutation              deleted / none
backend image build + Secret mount + restart health             passed
runtime UID / keyring source mode                               non-root match / 600
backend go test ./... / focused race / go vet ./...             passed
providersecrets coverage                                       81.7%
preflight tests / Compose config                               passed / passed
module / quality / security manual review                      passed
real provider requests / quota consumed                        0 / 0
```

The automated security scanner reported two pre-existing test-fixture strings
as possible hard-coded Keys; both are local `httptest` sentinels and neither is
a usable credential. No secret value, keyring content, ciphertext, database
URL, or provider body entered logs or committed files.

Rollback is asymmetric after the first vault write: this release can read old
BYOK rows, but the previous image cannot read new vault envelopes. Because this
slice did not mutate the formal provider rows, immediate rollback is still code
only. After production administrator saves begin, keep this image and keyring
available and restore a pre-cutover Postgres backup before reverting to the old
image. Do not remove the current keyring. F2.2 owns transactional bulk
backfill/rotation, ciphertext backup, and restart proof; F2.3 retains bounded
connection-test activation and final model-provider `.env` fallback removal.

## 2026-07-18 — G11.9F.2.2 Transactional backfill and rotation

Outcome: every formal model-provider ciphertext row now uses the current
Docker-Secret vault key. The new one-shot
`admin provider-secrets-rewrite` command defaults to dry-run and returns only
counts plus a deterministic plan SHA. Execute requires that exact SHA and a
verified pre-rewrite Postgres-backup SHA. It takes one Serializable transaction
and `SHARE ROW EXCLUSIVE` table lock, reads active and deleted rows in stable ID
order, validates every row before any update, then rewrites legacy BYOK and
retained-old-key vault envelopes. A plan/state/key change, malformed or copied
envelope, duplicate active User/Provider, missing retained key, SQL failure, or
oversized state aborts the whole transaction.

Deleted ciphertext rows are included so pruning an old key cannot leave hidden
dependencies. Empty rows remain empty. A well-formed custom legacy row that no
longer matches the stable BYOK private key is counted as blocked and prevents
execute. An unreadable active `SERVER_DEFAULT` legacy row may instead bind the
still-configured env fallback into the exact plan; vault failures never use
that fallback. The plan digest binds source ciphertext/config/deletion state,
action, active vault key, and a one-way digest of the fallback secret.

Stored vault and legacy envelopes are now parsed as bounded closed JSON:
unknown or trailing fields no longer disappear through permissive
`json.Unmarshal`. `rotate-provider-keyring.sh prepare` creates a new active key
while retaining old keys; `prune` creates an active-only candidate. Both
require user-owned mode-`600` input, mode-`700` non-symlink target parent,
canonical keys, a new target, and never print material. Postgres and MinIO
backup scripts now set `umask 077`.

The isolated `mm_chat_g119f22_test` proof covered one legacy row, one retained
old-key row, one current row, one empty row, and one deleted old-key row. Wrong
plan SHA changed nothing; the correct plan rewrote exactly three rows; a fresh
active-key-only Vault decrypted all four ciphertexts. A later unrecoverable
custom fixture produced `blocked_rows=1`, rejected execute, and preserved every
ref. The test database was deleted.

An isolated custom `pg_dump` was restored into a second database: all four
ciphertexts were `A256GCM` under the new active key, legacy count was zero, and
neither fixture plaintext nor keyring bytes appeared in logical dump output.
Both databases and temporary dumps were deleted; the formal migration
fingerprint remained 27/27 during that proof.

Formal cutover then ran behind a retained owner-only full Postgres dump and
restore drill. Initial dry-run found three rows: one decryptable legacy custom
provider, one unreadable historical `SERVER_DEFAULT` safely mapped to the
configured env fallback, and one empty deleted row. The exact plan executed two
updates atomically. Post-restart audit reported two current vault rows, one
empty row, `changed_rows=0`, and `blocked_rows=0`; backend and frontend remained
healthy and public runtime config still reported the model provider available.
The pre-rewrite dump/checksum remains under
`backup/provider-secrets-f22/` with mode `600` artifacts.

Verification:

```text
pure plan/action/hash/strict-envelope tests                 passed
providersecrets focused coverage                            83.1%
wrong-plan / blocked execute zero-write proof              passed / passed
live Postgres legacy/old/current/empty/deleted rewrite     passed
active-key-only Vault restart audit                        passed
isolated custom dump/restore + plaintext/keyring scan      passed
formal owner-only backup/checksum + restore drill          passed
formal exact-plan execute / final no-op audit              2 rows / passed
formal schema migration                                    none; version 27
keyring prepare/prune + no-overwrite tests                  passed
backend/frontend health                                    healthy / healthy
provider requests / quota                                  0 / 0
```

Rollback restores the paired pre-rewrite Postgres dump and prior keyring
selection before restarting backend; restoring only one side can strand
ciphertext. The provider env fallback remains configured and is not yet sole-
authority cleanup: F2.3 must perform bounded real connection-test activation,
then remove model-provider `.env` runtime fallback. No provider call entered
F2.2.

## 2026-07-19 — G11.9F.2.3 Connection-test activation and env removal

Outcome: model providers now require a bounded real model-list test before
activation, and Postgres plus the Docker-Secret-backed vault are the sole
runtime authority for model settings and credentials. The administrator API
adds `POST /v1/admin/providers/{id}/test` and `/activate`; the Provider page's
model refresh uses `test`, while enabling uses `activate`. Disabling remains a
direct metadata save.

The test allows at most 15 seconds, 2 MiB of response data, and 2048 normalized
models. Redirects and base URLs containing userinfo, query, or fragment are
rejected. OpenAI/OpenAI Compatible use Bearer `/models`; Gemini uses
`x-goog-api-key` `/v1beta/models`. A successful proof binds Provider ID,
canonical type, normalized base URL, and the exact vault envelope. Changing
type, endpoint, or Key clears the proof and disables the provider. Postgres
uses an expected-state conditional update so a concurrent administrator change
returns `PROVIDER_CONFIG_CHANGED` instead of committing a stale proof.

All runtime resolvers now fail closed for disabled or unattested rows. Public
config, live model listing, ordinary chat, image generation, and development
Knowledge answer-governance bootstrap use only valid Postgres/vault providers.
Image execution resolves `modelRef.providerId` dynamically; the old global
environment executor is gone. The unrelated legacy model-provider voice
executor was removed and Voice remains reserved for F.5.

The legacy model-provider variables were removed from Go/Next configuration,
examples, Compose, and preflight. After the formal provider passed activation,
the same values were removed from the gitignored `.env.single-server` and the
backend was force-recreated. Container environment-name inspection confirmed
that neither backend nor frontend contains them. `PROVIDER_TIMEOUT` and the
vault keyring settings remain infrastructure inputs.

Formal cutover was protected by an owner-only mode-`600` Postgres dump and
checksum under `backup/provider-activation-f23/postgres/`. The stored Server
Default passed activation with seven live model IDs. Public config remained
available, live model refresh returned seven IDs, a real `gpt-5.5` stream
completed with the exact requested sentinel, and an intentionally invalid
Bearer received HTTP 401 without modifying the stored Key. A real
`gpt-image-2` request resolved the same Postgres/vault provider, stored one
676237-byte PNG, and returned no redundant message; its test file was then
soft-deleted.

After `.env` cleanup and backend force-recreation, the original attestation
timestamp remained valid, model refresh still returned seven IDs, and a second
real chat stream completed with the exact restart sentinel. Both smoke
conversations were soft-deleted. The isolated
`mm_chat_g119f23_test` ciphertext reload proof passed and its database was
deleted.

Final cross-layer review closed three edge cases before commit: empty provider
model lists now serialize as `[]` rather than `null`; activation persistence
compares model IDs by ordered value rather than length alone; and dynamic image
resolver failures record sanitized unavailable audit metadata without prompt
or credential content. The final backend/frontend Compose images were rebuilt
and redeployed; every administrator model list then serialized as an array,
both containers were healthy and env-free, and a final real chat stream
completed before its test conversation was soft-deleted.

Release-gate review also fixed a pre-existing contradiction around stable BYOK
PEM input: Compose/Go accept a single-quoted one-line PEM with literal `\n`,
while preflight previously rejected every quote and backslash. Preflight now
admits only that field's bounded RSA PEM grammar and still rejects all other
quoted/escaped env assignments; positive and negative fixtures pass. The
current operator deployment intentionally uses local `build`, so it omits the
immutable `FRONTEND_IMAGE` required by the separate production-promotion gate;
the preflight test suite and current Compose rendering both pass.

Verification:

```text
backend go test ./... / focused race / go vet ./...             passed
frontend format / lint / typecheck / full Vitest               passed / passed / passed / 177 files, 844 tests
frontend production build                                      passed
preflight tests / current Compose config                       passed / passed
backend/frontend local Compose builds                           passed / passed
isolated Postgres vault reload / test database cleanup         passed / deleted
formal provider activation / live model refresh                passed / 7 models
real chat before restart / after restart / final image          passed / passed / passed
real gpt-image-2 artifact / cleanup                             image/png, 676237 bytes / deleted
invalid Bearer negative test                                   HTTP 401
administrator empty-model JSON contract                        [] / passed
backend/frontend legacy provider env names                     absent / absent
backend/frontend health after force-recreate                    healthy / healthy
quality/security scanners / staged secret scan                  passed / no real material
```

Rollback restores the retained pre-activation Postgres dump together with the
current vault keyring before starting an F2.2 image. If that older image needs
the former provider environment settings, recover them only from an
operator-owned secret source; do not reintroduce them to the F2.3 release.

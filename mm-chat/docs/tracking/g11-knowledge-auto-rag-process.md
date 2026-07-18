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

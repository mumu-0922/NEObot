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

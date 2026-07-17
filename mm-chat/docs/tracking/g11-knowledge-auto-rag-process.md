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

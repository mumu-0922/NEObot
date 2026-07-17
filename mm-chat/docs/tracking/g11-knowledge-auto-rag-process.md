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

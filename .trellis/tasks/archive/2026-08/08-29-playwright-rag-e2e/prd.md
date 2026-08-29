# Playwright RAG ingestion and citation journeys

## Goal

Extend the deterministic Chromium E2E layer to prove the user-visible RAG
journey from document upload through processing, retrieval, and cited answers
without calling a real parser, embedding service, retriever, or model Provider.

## What I already know

- The first Playwright foundation is merged and intercepts production
  same-origin `/mm-api` requests with per-test server-owned fixture state.
- This is the second E2E batch, before Memory and Skill/MCP coverage.
- The user-visible success contract is not merely listing a document: the
  document must become available, retrieval must hit it, and the answer must
  render a source citation.
- The suite must remain deterministic, parallel-safe, and quota-free.

## Assumptions

- Chromium remains the only browser in this batch.
- RAG worker internals stay covered by Python/Go tests; browser E2E owns the UI
  and `/mm-api` consumer contract.
- Existing product components will not receive E2E-only branches.

## Requirements

1. Create or select a knowledge collection and upload a deterministic text
   document through the real UI.
2. Model the server-owned processing lifecycle and verify the document reaches
   the available state after refresh/polling.
3. Attach/select the knowledge collection in a Conversation and submit a
   knowledge-grounded question.
4. Return deterministic retrieval evidence and a completed assistant answer;
   verify the answer exposes the correct document citation.
5. Cover a decisive failure path where document processing fails or retrieval
   is unavailable, and ensure the UI reaches a recoverable terminal state.
6. Fail the fixture on unhandled `/mm-api` request contracts.

## Acceptance Criteria

- [x] Playwright proves upload -> processing -> available using production UI.
- [x] Playwright proves selected collection -> retrieval hit -> cited answer.
- [x] A failure journey renders the actual user-facing terminal error/status.
- [x] Tests use inert local fixtures and never read real Provider secrets.
- [x] Existing 7 core E2E journeys and frontend gates remain green.
- [x] CI discovers the new specs through the existing `pnpm test:e2e` job.

## Definition of Done

- Playwright specs and reusable fixture contracts are implemented.
- Format, lint, typecheck, Vitest, Playwright, build, and proportional project
  verification pass.
- E2E docs/specs describe the newly covered RAG boundary.
- Verified work is committed; the Trellis task is archived and journaled.

## Out of Scope

- Real MinerU/native parsing, embeddings, PostgreSQL/pgvector, or reranking.
- RAG quality scoring and golden-dataset evaluation.
- Memory, Skill, and MCP browser journeys.
- Real Provider or external network smoke tests.

## Technical Notes

- Reuse `mm-chat/frontend/e2e/fixtures/NeoChatApiFixture` and the existing
  Playwright configuration.
- Keep knowledge/file fixture state in a dedicated helper instead of growing
  the core chat route fixture into a second monolith.
- Extend the conversation fixture to persist
  `config.selectedKnowledgeCollectionIds` across PATCH and subsequent reads.
- Extend completed fixture messages with `metadata.knowledge`; citations are
  only valid when the answer contains the matching marker such as `[K1]`.
- Implement three isolated journeys:
  1. upload a local TXT file, observe `处理中`, activate it in fixture state,
     refresh, and observe `可用`;
  2. select the collection for a conversation, send a grounded question, and
     verify an answer plus the expanded document citation;
  3. return `dependency_unavailable` with zero citations and verify the
     degraded, terminal user-facing banner.
- The file upload fixture returns the production `FileRecordDTO` contract and
  the knowledge fixture returns production collection/document DTO shapes.

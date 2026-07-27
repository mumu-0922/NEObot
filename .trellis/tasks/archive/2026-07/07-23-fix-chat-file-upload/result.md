# Chat attachment parsing and context injection result

## Outcome

- `d9c0d88` added bounded Go extraction for text, PDF, and Office attachments,
  preserved native image serialization, and injected escaped document content
  as untrusted current-turn context.
- `2433754` made attachment-only sends valid and preserved composer text and
  attachments until durable message acceptance.
- `b63bd7a` recorded the direct attachment contract and rollback boundary.
- OpenAI-compatible streams now accept a non-empty `finish_reason` followed by
  clean EOF while retaining markerless partial-EOF failure behavior.

## Acceptance evidence

- The task ledger already recorded all eleven acceptance criteria as complete,
  including the live `这是啥` plus `g18-rag-acceptance.txt` replay, native image
  regression, attachment-only send, failure restoration, PDF parsing, and the
  `finish_reason` EOF case.
- Parser and Handler tests cover the exact 20 MiB source boundary, per-file and
  combined prompt limits, archive complexity, unsupported content, prompt
  delimiter escaping, provider image preservation, and citation-free explicit
  failures.
- Frontend tests cover upload state, attachment-only requests, server message
  composition, and restoration before durable acceptance.

## Reconciliation verification — 2026-07-27

- `go vet ./...`: passed.
- Focused Go packages (`chat`, `knowledge`, `runtimeconfig`, `files`,
  `httpserver`, and `migration`): passed.
- Focused frontend regression run: `18 files / 196 tests` passed.
- The post-change full standalone gate passed on the same product tree:
  frontend `936 tests`, all backend tests/vet, and RAG
  `1,906 passed / 7 skipped`.
- The running frontend/backend/PostgreSQL/Redis/MinIO/RAG stack reported healthy
  during reconciliation.

## Rollback

Revert `b63bd7a`, `2433754`, and `d9c0d88` in reverse order. No schema or user
data migration is required.

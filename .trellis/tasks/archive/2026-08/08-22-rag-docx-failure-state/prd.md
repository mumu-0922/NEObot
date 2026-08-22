# Fix RAG DOCX Compatibility and Failure-State Projection

## Goal

Allow valid DOCX documents containing benign Word pagination markers to parse successfully, ensure terminal RAG failures become visible as terminal document/version states instead of remaining `processing`, and audit the existing knowledge corpus for equivalent inconsistencies.

## What I already know

* A valid 35,656-byte DOCX uploaded and bound successfully on 2026-08-22.
* The RAG Worker claimed the parse job and failed it within one second with `FORMAT_UNSUPPORTED`.
* Local reproduction traced the rejection to a run containing `w:lastRenderedPageBreak`.
* PostgreSQL retained `knowledge_processing_jobs.status='failed'` while the owning document/version remained `processing`/`uploaded`.
* The user deleted the affected document before implementation.
* Applied migrations are immutable; any database-function repair must be delivered through a new forward migration.

## Assumptions

* `w:lastRenderedPageBreak` is non-content metadata and can be ignored without changing extracted text or weakening active-content rejection.
* A terminal parse failure should project the pending version to `failed` with the same stable error code. The document remains `processing` by schema design when it has no active version; the UI derives the visible failure from the pending version.
* Existing successful/current document versions must remain available if only a replacement version fails.

## Open Questions

* None. Existing schema and repository contracts define replacement-version behavior.

## Requirements (evolving)

* Admit valid DOCX files containing `w:lastRenderedPageBreak` while preserving all existing fail-closed parser boundaries.
* Project terminal parse/embedding failures consistently into job and pending-version status while preserving the document lifecycle contract.
* Preserve the current version when a replacement/reprocess attempt fails.
* Surface terminal failure in the server DTO/UI rather than indefinite `processing`.
* Audit all existing knowledge documents/jobs for stuck, failed, or inconsistent states.
* Add focused Python, PostgreSQL/Go, and frontend regression coverage where the contract crosses layers.

## Acceptance Criteria (evolving)

* [x] A DOCX fixture containing `w:lastRenderedPageBreak` parses with the same extracted text as the equivalent fixture without the marker.
* [x] Active content, deleted revisions, unsupported run elements, and malformed OOXML still fail closed.
* [x] A terminal first-parse failure sets the pending version to failed with a stable public error code; the UI displays failed even though the parent document remains in its schema-valid `processing` state.
* [x] A terminal replacement failure does not discard the prior available version.
* [x] The knowledge UI renders failed state and does not label a terminal failure as processing.
* [x] Read-only audit reports whether any other current runtime documents have inconsistent status.
* [x] Focused tests, migration replay, Ruff/mypy/pytest, Go tests/vet, and frontend checks pass proportionally to the final diff.

## Definition of Done

* Tests added/updated at every changed contract boundary.
* Lint, typecheck, focused suites, migration replay, and relevant builds are green.
* RAG/storage contracts and operational notes are updated if behavior changes.
* Runtime images are rebuilt/restarted only after source verification; live state is re-audited afterward.
* A focused commit is created without pushing.

## Out of Scope

* Supporting legacy binary `.doc` files.
* Broadening DOCX support to active content, embedded OLE objects, deleted revisions, or arbitrary unknown OOXML.
* Reprocessing or restoring the user-deleted document.
* Changing embedding, retrieval, reranking, or citation semantics.

## Technical Notes

* Parser: `mm-chat/rag/src/mm_chat_rag/offline_parser/native/docx.py`.
* Parser tests: `mm-chat/rag/tests/unit/test_parser_native_docx.py`.
* Processing state authority lives in PostgreSQL migration functions and is called by the Python Worker repository.
* Frontend page: `mm-chat/frontend/src/components/knowledge/ServerKnowledgeBase.tsx`.
* Existing runtime evidence: job `failed/FORMAT_UNSUPPORTED`, version `uploaded`, document `processing`.
* Runtime audit found 55 active/succeeded documents and 9 historical G7.8 Smoke documents with terminal failed parse jobs but uploaded versions. No active user document is inconsistent.

## Decision (ADR-lite)

**Context**: The failure spans a strict Python parser, a PostgreSQL-owned job state transition, and a frontend that renders only the parent document status.

**Decision**: Ignore only the exact non-content `w:lastRenderedPageBreak` run child; repair `knowledge_claim_processing_job` and `knowledge_finish_processing_job` in forward migration 104 so terminal parse/embedding failures atomically mark eligible pending versions failed; backfill equivalent historical rows; derive the visible UI badge from a failed pending version only when there is no active current version.

**Consequences**: Text extraction stays deterministic and fail-closed for unknown/active OOXML. Active documents survive failed replacement attempts. Historical failed jobs become reprocessable without deleting their documents. The parent document schema remains unchanged.

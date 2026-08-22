# Fix short RAG overlap rejection

## Goal

Restore valid DOCX ingestion when structure chunk planning cannot assemble the
preferred 60-token overlap from whole source atoms. The ingestion pipeline must
keep its exact-source lineage contract without rejecting the entire document.

## Requirements

- Preserve the frozen structure chunk profile and its active generation hash.
- Emit `window_overlap` only when exact prior-child fragments form a 60–100
  token window.
- When exact atom reuse yields fewer than 60 tokens, omit overlap for that one
  child transition instead of emitting an invalid artifact or rejecting the
  document.
- Keep Native and MinerU artifact mappers fail-closed for malformed explicit
  overlap metadata.
- Add deterministic regression coverage for short overlap candidates and the
  production artifact-building path.
- Build and deploy only the changed RAG worker through an immutable image tag.
- Reprocess the affected user document and verify it reaches `active` after
  parse plus passage embedding.
- Audit the other current documents in the `test` collection for non-active
  states or terminal job errors.

## Acceptance Criteria

- [x] Planner output contains no `window_overlap` with 1–59 tokens.
- [x] A short-overlap structure fixture builds projection artifacts without
      `NATIVE_STRUCTURE_ARTIFACT_INVALID`.
- [x] Focused Ruff, mypy, and pytest checks pass for changed RAG code.
- [x] The rebuilt RAG container is healthy and pinned to an immutable tag.
- [x] The affected DOCX completes parse and passage embedding and becomes
      `active`.
- [x] The other three current documents in `test` remain `active`.

## Definition of Done

- Tests and executable contracts are updated where needed.
- No active generation/profile hash changes.
- Deployment and rollback image identities are recorded.
- The source fix is committed; runtime state and user files are not committed.

## Technical Approach

Add a shared planner-level overlap minimum constant and make
`_overlap_suffix(...)` return no overlap when whole-fragment reuse cannot meet
that minimum. Keep both artifact mappers' explicit overlap validation aligned
with the shared constant. Cover the planner invariant plus a Native artifact
regression, then deploy the RAG worker and use the existing reprocess path for
the exact failed document.

## Decision (ADR-lite)

**Context:** The planner currently emits exact prior-child atoms totaling
38/51/56/58 tokens, while both artifact mappers reject any explicit overlap
below 60 tokens. Splitting an earlier source atom only for overlap would break
the existing exact-fragment reuse/lineage contract.

**Decision:** Omit overlap for the exceptional transition when exact whole-atom
reuse cannot reach 60 tokens within the 100-token maximum.

**Consequences:** The affected transition has lower retrieval-context reuse but
the source remains indexable and all emitted overlap metadata stays truthful.
The active chunk profile hash remains unchanged.

## Out of Scope

- Rebalancing the frozen tokenizer atomization algorithm.
- Changing Parent/Child target sizes or active generation hashes.
- Repairing historical G7.8 smoke fixtures unrelated to the current user
  collection.
- Changing frontend status rendering or PostgreSQL failure projection already
  repaired by migration 104.

## Technical Notes

- Runtime failure: `NATIVE_STRUCTURE_ARTIFACT_INVALID` at
  `native_structure_artifacts.py::_chunk_fragments`.
- Production fixture produced short exact overlaps of 58, 56, 38, and 51
  tokens; source parsing and source-hash binding both succeeded.
- Relevant code:
  `mm-chat/rag/src/mm_chat_rag/structure_chunking.py`,
  `mm-chat/rag/src/mm_chat_rag/native_structure_artifacts.py`, and
  `mm-chat/rag/src/mm_chat_rag/mineru_structure_artifacts.py`.
- Relevant contract: `mm-chat/docs/contracts/rag-structure-chunking.md`.
- Deployment contract:
  `.trellis/spec/operations/runtime-recreate-image-pinning.md`.

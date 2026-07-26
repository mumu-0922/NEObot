# Improve Knowledge citation display

## Goal

Replace raw Knowledge citation UUIDs and serialized locator JSON with a stable,
localized source label that works across the currently indexed document formats
without weakening Citation authority or requiring reindexing.

## What I already know

- The live card currently renders a shortened `documentId` because the durable
  `RAGCitation` payload does not include the already-authorized `SourceName`.
- The live locator is a versioned `g7.4-locator-summary.v1` wrapper with a
  `primary` locator and fragments. The UI only probes legacy top-level
  `page/sheet/cell/section` fields, then falls back to `JSON.stringify`, which
  exposes internal locator structure.
- Hydrated evidence already contains a validated, ACL-authorized `SourceName`,
  so no new database read, migration, or reindex is required.
- Current canonical locator kinds include `page_bbox`, `slide_shape`,
  `sheet_cell`, `line_range`, `ooxml_part_xpath`, and `text_offset`.
- Page, slide, and line indexes are zero-based canonical coordinates. Sheet
  names are deliberately opaque in the current locator contract; only the cell
  range is safe and human-readable.

## Requirements

- Add a bounded `sourceName` to newly minted Knowledge Citations from the
  already reauthorized `HydratedEvidence.SourceName`.
- Keep IDs, hashes, version identity, and raw locator in the payload for
  Citation authority and compatibility, but never render those internal values
  as the user-facing source title.
- Normalize the versioned locator summary at the Citation boundary into an
  additive, typed display locator containing only human-safe coordinates.
- Support localized display for:
  - `page_bbox` -> one-based page number;
  - `slide_shape` -> one-based slide number;
  - `sheet_cell` -> A1 cell or range without the opaque sheet hash;
  - `line_range` -> one-based line or range.
- Treat `ooxml_part_xpath`, `text_offset`, malformed, unknown, or incomplete
  locators as having no display location. The card must show the source name
  only and must never stringify raw JSON.
- Preserve legacy top-level `page`, `sheet/cell`, and `section` rendering for
  old messages where those values are already human-readable.
- Old messages without `sourceName` must fall back to a localized generic
  Knowledge source label, not UUIDs, hashes, citation IDs, or raw JSON.
- Render source names as normal React text, trim control characters, enforce a
  display bound, and preserve the snippet and `[K#]` marker.
- Add English, Chinese, and Japanese copy for the generic source and supported
  display locator kinds.
- Do not change retrieval, ranking, Citation marker reconciliation, ACL,
  governance, or answer generation behavior.

## Acceptance Criteria

- [x] A new PDF Citation displays `[K1] <filename> · 第 N 页` and no UUID/JSON.
- [x] PPTX displays a one-based slide; XLSX displays a bounded cell/range;
      text-like sources display a one-based line/range.
- [x] DOCX/unknown locator kinds display the filename without leaking opaque
      part hashes, XPath payload references, offsets, or locator JSON.
- [x] A legacy Citation without `sourceName` displays a localized generic label
      and remains usable after message reload.
- [x] Malformed or hostile source names/locators cannot inject markup or make
      the card fail; they degrade to the generic label/no locator.
- [x] Citation IDs, hashes, raw locators, snippets, ranking, and `[K#]`
      reconciliation remain unchanged in the durable authority payload.
- [x] Existing Direct, Knowledge, Web, and Both routing tests do not regress.

## Definition of Done

- Backend table-driven tests cover source-name projection and display-locator
  normalization for every supported and fallback kind.
- Frontend tests cover metadata normalization, localized presentation inputs,
  legacy messages, malformed inputs, and the no-raw-JSON invariant.
- `gofmt`, backend tests, frontend format/lint/typecheck/tests/build pass.
- No migration, reindex, public object URL, or additional per-Citation API call
  is introduced.
- Rollback is additive: old clients ignore the new fields and reverting the UI
  returns to the previous card without changing stored evidence authority.

## Technical Approach

1. Extend `RAGCitation` with `sourceName` and an optional structured
   `displayLocator` DTO.
2. At mint time, unwrap `g7.4-locator-summary.v1.primary.locator`, validate the
   recognized kind, convert zero-based coordinates to one-based display
   numbers, and omit opaque/unsupported fields.
3. Extend the frontend Knowledge Citation type and untrusted metadata
   normalizer to accept only the bounded additive fields.
4. Move Citation title/location formatting into a pure tested formatter. The
   React card consumes its result and never serializes the raw locator.
5. Keep compatibility parsing for human-readable legacy top-level locators.

## Decision (ADR-lite)

**Context**: Formatting by file extension is brittle and the frontend cannot
derive a trustworthy filename from a document UUID. Fetching document metadata
per Citation would add N+1 requests and fail for historical/revoked state.

**Decision**: Project the already-authorized source name and a minimal typed
display locator into the durable Citation at mint time. Preserve the raw
locator only as internal authority data and use safe fallback rendering.

**Consequences**: New messages become friendly without a schema migration or
reindex. Old messages cannot recover filenames retroactively, so they receive a
generic localized label. Opaque sheet names and OOXML section paths remain out
of scope until the indexing contract carries authorized human labels.

## Out of Scope

- Reindexing old documents or rewriting historical message metadata.
- Reconstructing sheet names, DOCX headings, or OOXML paths from opaque hashes.
- Citation preview, click-to-open, or version-aware source download.
- Changing Knowledge retrieval, ranking, routing, or Citation authority.

## Technical Notes

- Backend Citation minting: `mm-chat/backend/internal/chat/rag_citation.go`
- Hydrated source contract:
  `mm-chat/backend/internal/knowledge/evidence_postgres.go`
- Frontend normalization: `mm-chat/frontend/src/lib/knowledge/citations.ts`
- Frontend display:
  `mm-chat/frontend/src/components/knowledge/KnowledgeEvidenceBlock.tsx`
- Frontend types: `mm-chat/frontend/src/lib/knowledge/types.ts`
- Current locator projection: `mm-chat/rag/src/mm_chat_rag/projection.py`
- Research: [`research/current-citation-contract.md`](research/current-citation-contract.md)

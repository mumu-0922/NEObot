# Current Knowledge Citation display contract

## Runtime cause

`KnowledgeEvidenceBlock.tsx` chooses `documentId`, then `collectionId`, then the
Citation ID as its source title. Its locator formatter only recognizes legacy
top-level `page`, `sheet/cell`, and `section`; all other objects are serialized
with `JSON.stringify`.

The active RAG path stores `knowledge_child_search_projections.locator_summary`,
which is a `g7.4-locator-summary.v1` wrapper:

```json
{
  "schemaVersion": "g7.4-locator-summary.v1",
  "primary": {
    "kind": "line_range",
    "locator": {
      "kind": "line_range",
      "startLine": 0,
      "endLine": 10
    }
  },
  "fragments": []
}
```

That shape mismatch explains the live UUID plus raw JSON display.

## Existing authorized data

`knowledge.HydratedEvidence` already contains `SourceName`. The hydration query
returns it from the SECURITY DEFINER authorization boundary and rejects empty,
oversized, newline, NUL, or otherwise invalid names before evidence reaches
Citation minting. `RAGCitation` currently drops that field.

## Current locator kinds

The projection prefers these canonical display candidates:

1. `page_bbox`: zero-based page plus geometry.
2. `sheet_cell`: opaque sheet hash plus `startCell`/`endCell`.
3. `slide_shape`: zero-based slide plus opaque shape identity.
4. `ooxml_part_xpath`: opaque part hash and path reference.
5. `line_range`: zero-based source line range.
6. `text_offset`: structural fallback offset.

Only page, slide, cell range, and line range contain bounded coordinates useful
to an end user. Opaque hashes, part identities, XPath references, geometry, and
text offsets must not be rendered.

## Chosen boundary

Mint an additive `sourceName` and minimal typed `displayLocator` alongside the
existing immutable Citation authority fields. This avoids an N+1 document
lookup, remains valid after reload, needs no database migration or reindex, and
lets old clients ignore the new fields.

Frontend fallback order:

1. Valid `sourceName` plus localized typed `displayLocator`.
2. Valid `sourceName` alone.
3. Localized generic Knowledge source label for historical messages.

Raw UUIDs, hashes, and serialized locator JSON are never a display fallback.

# Curate and Localize Skill Marketplace Categories

## Goal

Replace the raw 296-entry LobeHub tag dump with a stable, useful category rail
that matches the MCP Marketplace quality: a bounded set of high-signal
categories, localized labels, and a distinct semantic icon for each category.

## Requirements

- Expose only the 21 established high-volume LobeHub top-level categories in a
  stable product-defined order.
- Keep `All` and keyword search as the escape hatch for Skills whose raw tags
  are not in the curated rail.
- Keep category filtering server-authoritative by forwarding the exact selected
  canonical LobeHub category ID; never filter only the currently loaded page.
- Reject non-curated category query values before upstream I/O.
- Render localized category names for Chinese, English, and Japanese.
- Render a distinct Lucide icon appropriate to every curated category.
- Do not change Skill cards, installation, detail, paging, source tabs, or the
  existing same-origin Skill avatar path.
- Use focused verification only; do not run full repository/component suites.

## Acceptance Criteria

- [x] Chinese UI shows readable Chinese category names rather than raw slugs.
- [x] The category rail contains at most 21 curated entries plus `All`.
- [x] Every curated entry has its own semantic icon instead of a generic folder.
- [x] Clicking a category still issues a backend search using its exact
  canonical LobeHub category ID.
- [x] Raw community categories such as `_meta` and `Backend & Infrastructure`
  never enter the category DTO.
- [x] Unknown category query values fail before Marketplace transport.
- [x] Focused backend/frontend tests, format, lint, typecheck, build, and
  deployed health checks pass.

## Definition of Done

- Regression tests cover ordering/filtering, non-curated request rejection,
  localization keys, and category icon composition.
- Backend/frontend Skill Store specs record the curated taxonomy boundary.
- Frontend and backend images are rebuilt and deployed healthy.
- Work is committed, archived, and journaled.

## Technical Approach

The backend owns one ordered allowlist of 21 exact upstream IDs. It projects
only matching category/count rows and rejects an explicit filter outside the
allowlist. The frontend owns presentation metadata for the same IDs: Lucide
icons and `next-intl` keys. Counts and search results remain live LobeHub data.

## Decision (ADR-lite)

**Context**: LobeHub returns 296 categories. The first 32 are coherent official
top-level domains; the tail includes low-count user-defined labels, mixed case,
underscores, duplicates by meaning, and bundle metadata. Rendering all of them
is source leakage rather than usable navigation.

**Decision**: Curate 21 exact high-volume categories that cover core Neo Chat
use cases, retain `All`/search for the long tail, and localize only the display
label while keeping exact source IDs as API authority.

**Consequences**: Navigation is stable and readable. Newly added upstream
categories do not appear automatically until deliberately admitted, but their
Skills remain discoverable through `All` and search.

## Out of Scope

- Aggregating several upstream categories into one synthetic multi-query group.
- Translating Skill names/descriptions.
- Inventing unique per-Skill artwork when LobeHub supplies only an author icon.
- Changing the LobeHub API or Marketplace credentials.
- Full standalone/full component test suites.

## Technical Notes

- Backend owner: `mm-chat/backend/internal/skillsupply/marketplace.go`.
- Frontend owner:
  `mm-chat/frontend/src/components/skills/SkillMarketplacePrimitives.tsx`.
- Locale files: `mm-chat/frontend/src/i18n/locales/{zh,en,ja}/SkillStore.json`.
- The selected IDs are exact live LobeHub IDs, so one category remains one
  upstream query rather than a client-side approximation.

## Research References

- [`research/lobehub-category-taxonomy.md`](research/lobehub-category-taxonomy.md)
  — sanitized live counts and taxonomy decision.

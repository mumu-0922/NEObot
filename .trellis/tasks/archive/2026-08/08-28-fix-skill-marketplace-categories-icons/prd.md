# Fix Skill Marketplace Categories and Icons

## Goal

Restore the live LobeHub category rail and real Skill icons so the Skill
Marketplace reaches the same browsing quality as the MCP Marketplace without
changing the existing installation or trust boundaries.

## Requirements

- Display every valid category returned by the configured LobeHub Skill API.
- Display an upstream HTTPS Skill icon when present, with the existing safe
  local fallback when the image is missing or fails to load.
- Keep search, sort, detail, direct-link install, and source tabs unchanged.
- Keep category/source failures fail-open for the item list.
- Keep all existing input bounds and URL validation; only raise the category
  collection bound enough to accommodate the current upstream catalog.
- Use focused verification only; do not run the full repository gate.

## Acceptance Criteria

- [x] The live Skill Marketplace renders more than the synthetic `All`
  category when LobeHub returns categories.
- [x] Skill cards render their real HTTPS icon instead of a first-letter
  placeholder, and fall back safely after an image error.
- [x] A category response containing the current 296 entries is accepted,
  while an unreasonably large or malformed response still fails closed.
- [x] Focused backend and frontend regressions pass.
- [x] Frontend typecheck and production build pass.
- [x] The rebuilt frontend/backend are healthy after deployment.

## Definition of Done

- Focused tests cover the category ceiling and icon rendering contract.
- Formatting, lint for changed files, typecheck, and build are green.
- Relevant Skill supply-chain/frontend specs record the discovered upstream
  cardinality and icon presentation rule.
- Changes are committed and the Trellis task is archived.

## Technical Approach

Increase the Skill category response ceiling from 256 to a separately named,
bounded value that admits the observed 296-entry catalog. Mirror the proven MCP
icon presentation while routing the allowlisted GitHub avatar through the
same-origin Next.js image optimizer; preserve short-text, lazy loading,
`no-referrer`, error fallback, and dark-mode sizing behavior.

## Decision (ADR-lite)

**Context**: The production LobeHub category endpoint currently returns 296
entries, while Neo Chat rejects any response over 256. The Skill UI also
discarded the validated icon DTO and always rendered the first letter.

**Decision**: Preserve the upstream taxonomy rather than inventing local
categories, raise only the bounded category cardinality, and use a Skill-owned
icon renderer backed by the same-origin Next.js image optimizer.

**Consequences**: The category rail can be long but remains scrollable and
faithful to the source. The browser requests icons only from its own origin;
the server-side optimizer may fetch the narrowly allowlisted GitHub avatar URL.

## Out of Scope

- Redesigning or merging the LobeHub taxonomy.
- Adding a new image proxy/cache service.
- Changing Skill package installation, admission, or execution.
- Running the full standalone or full component test suites.

## Technical Notes

- Backend category parser:
  `mm-chat/backend/internal/skillsupply/marketplace.go`.
- Skill card presentation:
  `mm-chat/frontend/src/components/skills/SkillMarketplacePrimitives.tsx`.
- Existing shared behavior source:
  `mm-chat/frontend/src/components/mcp/McpServerIcon.tsx`.
- Live M2M inspection on 2026-08-28 returned 296 `{category,count}` rows and
  HTTPS GitHub icon URLs for the first-page Skill summaries.

## Research References

- [`research/lobehub-live-shape.md`](research/lobehub-live-shape.md) — sanitized
  live response shape and root-cause evidence.

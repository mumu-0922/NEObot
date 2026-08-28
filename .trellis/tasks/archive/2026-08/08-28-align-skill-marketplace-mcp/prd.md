# Align Skill Marketplace with MCP and Fix LobeHub Detail

## Goal

Make the LobeHub Skill Marketplace use the same product structure and interaction
model as the existing MCP Marketplace, while repairing the confirmed LobeHub
detail-request authentication mismatch.

## What I Already Know

- The current Skill page keeps the correct top-level `Installed | Skill Store`
  separation, but its Store body reuses the old fixed list/detail split.
- The desired Store body is the proven MCP Marketplace pattern: top search,
  category rail, card grid, incremental pagination, and an item detail overlay.
- Authenticated LobeHub Skill list/search succeeds.
- An authenticated request to `/api/v1/skills/{identifier}` returns
  `401 invalid_token`, while the same exact detail URL without Authorization
  returns `200` and the complete Skill detail DTO.
- Exact-version package download must remain authenticated and must continue
  through Neo Chat's immutable owner-private Skill supply chain.

## Requirements

- Rebuild only the LobeHub Store body to match the MCP Marketplace information
  architecture and responsive behavior.
- Keep `Installed | Skill Store`, the OpenAI Curated source, and direct-link
  installation available.
- Put search across the top; render categories/counts on the left and a
  responsive two-column Skill card grid on the right.
- Load detail in an overlay/dialog rather than replacing the grid with a fixed
  right pane; closing restores focus to the originating card.
- Preserve category, query, sort, pagination, locale, loading, retry, and
  source-isolated failure state.
- Use bounded public GET for exact Skill detail only.
- Keep authenticated M2M for Skill list/categories and exact-version package
  download.
- Keep strict DTO validation, exact SemVer re-resolution, archive validation,
  fingerprinting, owner-private installation, and no-execute policy unchanged.
- Add regression coverage proving public detail has no Authorization header and
  authenticated list/download behavior remains unchanged.
- Deploy backend/frontend and smoke-test the live Store.

## Acceptance Criteria

- [x] LobeHub Store visually follows the MCP Marketplace layout shown by the
      owner: top search, category rail, card grid, external-source link.
- [x] Selecting `bytedance-deer-flow-find-skills` opens a populated detail layer
      instead of `skill source is unavailable`.
- [x] Detail requests omit Authorization; list/category/download requests retain
      the existing M2M boundary.
- [x] Detail close/back restores card focus and does not discard the current
      result grid.
- [x] Search, category, sorting, pagination, locale, retry, direct-link install,
      OpenAI Curated, and Installed management keep working.
- [x] Focused and full frontend/backend tests, production build, security and
      Skill supply-chain gates pass.
- [x] Updated containers are healthy and live smoke checks pass.

## Failure and Edge Cases

- A detail failure remains scoped to the selected item and does not replace the
  entire Marketplace grid.
- Rapid selection/search changes abort or ignore stale detail responses.
- Closing the detail layer during an in-flight request does not reopen it.
- Unsupported identifiers, malformed DTOs, and install drift remain fail-closed.
- Public detail transport keeps the existing HTTPS origin, redirect, timeout,
  and response-size boundaries.

## Out of Scope

- Combining MCP and Skill into one resource type.
- Changing the Installed library or per-conversation Skill-selection model.
- Adding arbitrary webpage, command, ZIP, or unauthenticated package download.
- Redesigning the MCP Marketplace itself.

## Technical Notes

- MCP UX reference: `mm-chat/frontend/src/components/mcp/McpMarketplace.tsx`.
- Skill Store implementation: `mm-chat/frontend/src/components/skills/`.
- LobeHub transport: `mm-chat/backend/internal/mcpclient/marketplace_lobehub.go`.
- Confirmed upstream behavior is recorded in
  `research/lobehub-detail-auth-and-mcp-parity.md`.

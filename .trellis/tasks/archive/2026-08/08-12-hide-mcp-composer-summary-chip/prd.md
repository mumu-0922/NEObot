# Compact MCP Composer and Add Installed Icons

## Goal

Keep the composer compact by showing only the MCP Tools wrench button. Remove
the adjacent inherited/server-name summary chip such as `Context7 平台…` while
preserving all selection, status, tooltip, dialog, and send-admission behavior.
Make Installed Server cards visually consistent with Marketplace cards by
showing each Server's normalized icon or a stable MCP fallback.

## Requirements

- The composer renders the existing wrench button with its current enabled,
  disabled, attention, tooltip, accessibility, and click behavior.
- The composer does not render inherited or enabled-server name chips beside
  the wrench at any viewport width.
- The Tools dialog and management page continue to show authoritative server,
  selection, and Tool details.
- Installed Server cards render a bounded HTTPS/short-text Marketplace icon
  when available and a generic MCP fallback otherwise.
- New Marketplace installs persist only the already-normalized icon display
  value in private Server metadata; no browser-supplied icon is accepted.
- Existing reviewed stdio installs obtain their icon from the current exact
  manifest artifact, so Context7 does not require reinstalling.
- No MCP selection, authorization, or execution behavior changes.

## Acceptance Criteria

- [x] Only the wrench is visible in the composer Tools area.
- [x] Clicking the wrench still opens the Tools dialog.
- [x] The wrench tooltip still reports the current server/Tool summary.
- [x] Context7 shows its Marketplace icon in Installed without reinstalling.
- [x] Servers without an icon still show a generic MCP fallback.
- [x] Unsafe/non-HTTPS icon URLs never render as remote images.
- [x] Focused MCP Tools UI regression, lint, formatting, and typecheck pass.

## Definition of Done

- Focused backend/frontend tests cover icon provenance, DTO normalization,
  fallback rendering, and the icon-only composer contract.
- Relevant backend/frontend MCP specifications and API contract are synchronized.
- Backend and frontend images are built and deployed for manual verification.

## Technical Approach

Remove only the composer-only summary-chip render branch in
`McpToolsControl.tsx`. Retain `statusLabel` because it remains authoritative
content for the wrench tooltip and Tools panel. Extend the public Server DTO
with optional normalized `icon`, persist the Marketplace adapter's bounded
value in existing private metadata, and rebind existing stdio display icons
from the current exact manifest artifact. Extract one shared icon renderer for
Marketplace and Installed cards, including HTTPS/no-referrer handling and a
generic fallback. Add focused regression coverage across both layers.

## Decision (ADR-lite)

**Context**: The adjacent server-name chip duplicates information available
through the selected wrench state, tooltip, and dialog while consuming composer
space.

**Decision**: Use an icon-only composer control and retain detailed status in
the tooltip/dialog. Treat installed icons as display-only metadata: Marketplace
installs use the backend-normalized value, reviewed stdio installs rebind to the
manifest, and all other Servers use a local fallback.

**Consequences**: The composer is cleaner. Server names are one interaction
away instead of permanently visible, but selection authority and discoverability
remain unchanged. Installed cards gain visual identity without letting icons
become execution or trust authority.

## Out of Scope

- Changing MCP selection, authorization, or execution behavior.
- Removing server details from the Tools dialog or management page.
- Changing the wrench icon, colors, tooltip, or accessibility label.
- Fetching Marketplace detail during ordinary installed-list requests.

## Technical Notes

- Owning component: `mm-chat/frontend/src/components/mcp/McpToolsControl.tsx`.
- Focused regression: `mm-chat/frontend/src/__tests__/mcpToolsControl.test.tsx`.
- Backend authority: `mm-chat/backend/internal/mcpclient/` and
  `mm-chat/mcp/manifest.json`.
- Contracts: `.trellis/spec/{backend,frontend}/mcp-tools.md` and
  `mm-chat/docs/contracts/mcp-tools-api.md`.

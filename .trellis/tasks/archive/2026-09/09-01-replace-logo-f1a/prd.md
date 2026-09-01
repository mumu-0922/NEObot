# Replace Frontend Logo with F1-A

## Goal

Replace the current frontend logo with the user-selected F1-A mark: a balanced
three-ribbon triangular knot using the existing soft aqua, sky-blue, lavender,
and pink-violet brand palette.

## Requirements

- Implement F1-A as a transparent, vector-first SVG mark suitable for light and
  dark application surfaces.
- Replace the inline React `Logo` mark without changing its public component API.
- Replace public raster and favicon assets consumed by browser metadata, SEO,
  Apple/PWA installation, and existing external references.
- Provide 192px and 512px PWA PNG assets and a multi-compatible favicon.
- Keep the `NeoBot` wordmark, internal package names, APIs, storage, Docker,
  backend, RAG, and runtime data unchanged.
- Preserve accessibility behavior and avoid adding network-dependent logo
  rendering.

## Acceptance Criteria

- [ ] Sidebar and welcome screen render the F1-A mark through the existing
      `Logo` component.
- [ ] `/logo.svg`, `/logo.png`, `/logo-192.png`, and `/logo-512.png` contain the
      F1-A mark with transparent backgrounds.
- [ ] App metadata and manifest reference the new vector/raster assets with
      correct MIME types and sizes.
- [ ] `src/app/favicon.ico` contains the F1-A mark.
- [ ] Focused logo/metadata tests, formatting, lint, and type-check pass.
- [ ] No internal runtime or persistence identifiers change.

## Definition of Done

- Visual assets are generated deterministically from the vector master.
- Focused frontend checks pass.
- The change is committed as one focused logo replacement commit.

## Technical Approach

Create a public SVG master using three rotated, rounded ribbon paths with the
selected translucent palette. Mirror the same vector geometry in the existing
inline React `Logo` component so current callers remain unchanged. Render PNG
and ICO derivatives mechanically from the SVG using the locally installed
Playwright browser runtime; do not crop the AI comparison board into production
assets.

## Decision (ADR-lite)

**Context:** The selected ImageGen concept is a raster preview on white, while
the product needs transparent, scalable assets that work in dark mode and at
favicon size.

**Decision:** Reconstruct F1-A as deterministic SVG geometry and generate all
raster derivatives from that vector master.

**Consequences:** The production logo is crisp and reproducible at every size.
It follows the approved visual concept without depending on generated-image
background removal or an opaque white tile.

## Out of Scope

- Changing the `NeoBot` product name or wordmark styling.
- Updating historical product screenshots.
- Renaming internal engineering identifiers or deployment resources.
- Backend, RAG, database, Memory, Skill, or MCP changes.

## Technical Notes

- Existing inline mark: `mm-chat/frontend/src/components/ui/Icons.tsx`.
- Existing public image: `mm-chat/frontend/public/logo.png`.
- Browser favicon: `mm-chat/frontend/src/app/favicon.ico`.
- Metadata consumers: `mm-chat/frontend/src/app/layout.tsx`,
  `mm-chat/frontend/src/app/manifest.ts`, and `mm-chat/frontend/src/lib/seo.ts`.
- Approved visual reference:
  `/home/mumu/.codex/generated_images/01a0287b-60fb-74b0-9510-2953c5363285/exec-632b085f-8bfc-45c1-8868-db5eef6cdc08.png`.

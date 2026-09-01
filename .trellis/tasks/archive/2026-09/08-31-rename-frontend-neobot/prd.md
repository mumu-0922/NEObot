# Rename Frontend Brand to NeoBot

## Goal

Rename the user-facing frontend product brand from `Neo Chat` to `NeoBot`
without changing internal engineering identifiers, persistence keys, APIs,
deployment resources, or runtime data.

## Requirements

- Display `NeoBot` anywhere the frontend presents the product name to users.
- Update browser metadata, SEO content, Open Graph content, JSON-LD, and the PWA
  manifest to use `NeoBot`.
- Update English, Chinese, and Japanese user-facing translations containing the
  old product name.
- Update frontend loading and error surfaces containing the old product name.
- Keep the existing visual logo asset unchanged.
- Preserve all internal identifiers required for compatibility, including
  `mm-chat`, `NeoChatApiClient`, `neo-chat-*` storage/event/schema keys, MIME
  identifiers, import-package identifiers, Cloudflare project naming, tests and
  fixture class names.

## Acceptance Criteria

- [x] The rendered application wordmark displays `NeoBot`.
- [x] Login, account-security, MCP, loading, error, and accessibility copy no
      longer presents `Neo Chat` as the product name.
- [x] Browser title, application metadata, Open Graph metadata, JSON-LD, and PWA
      manifest identify the product as `NeoBot`.
- [x] Existing logo imagery remains unchanged.
- [x] Internal compatibility identifiers remain unchanged.
- [x] Focused frontend tests, format check, lint, and type-check pass.

## Definition of Done

- Focused tests cover the brand metadata and visible copy where practical.
- Prettier, ESLint, and TypeScript checks pass for the affected frontend.
- The change is committed as one focused branding commit.

## Technical Approach

Use the existing `SITE_NAME` export as the canonical metadata brand and update
localized visible strings at their current translation boundaries. Do not run a
blind repository-wide replacement because many `NeoChat` and `neo-chat`
occurrences are durable internal contracts.

## Decision (ADR-lite)

**Context:** The external product is being renamed while its deployed runtime
and stored browser/server state must remain compatible.

**Decision:** Change only presentation-layer brand strings and metadata. Retain
all internal names and the current logo asset.

**Consequences:** Users see the new brand immediately with no migration. Some
internal code symbols and keys continue to contain the legacy name by design.

## Out of Scope

- Renaming the repository or `mm-chat/` product directory.
- Renaming npm packages, Go modules, Python packages, API client types, fixtures,
  or internal CSS class names.
- Renaming Docker images, containers, networks, volumes, environment variables,
  database identifiers, storage keys, events, MIME types, or import schemas.
- Redesigning or regenerating the logo, favicon, screenshots, or other artwork.
- Changing backend or RAG behavior.

## Technical Notes

- Primary metadata source: `mm-chat/frontend/src/lib/seo.ts`.
- App metadata/PWA consumers: `mm-chat/frontend/src/app/layout.tsx` and
  `mm-chat/frontend/src/app/manifest.ts`.
- Main wordmark source: localized `ChatApp.productName` strings.
- User-visible localized brand references exist in `AccessPassword`,
  `AccountSecurity`, `Sidebar`, `Mcp`, and `ChatApp` locale files.
- Internal compatibility references were identified by a scoped `rg` search and
  are intentionally excluded.

# Integrate LobeHub Skill Marketplace and External Installs

## Goal

Turn the existing Skill Store into a real multi-source supply surface: browse,
search, inspect, and install version-pinned Skills from LobeHub Marketplace,
while preserving the existing OpenAI curated collection and allowing an owner
to install a Skill from an explicit external source link.

## What I Already Know

- The current Skill page already separates `Installed` from `Skill Store`, but
  the Store catalog is fixed to `openai/skills/skills/.curated`.
- Neo Chat already has a backend-only LobeHub Marketplace client with M2M token
  ownership for MCP and an exact-version Skill ZIP download method.
- LobeHub's official Skill API exposes paginated list/search, categories,
  detail/version history, and exact-version ZIP download endpoints.
- LobeHub requires a bearer token; the browser must not call it directly and
  the backend must not shell out to `npx @lobehub/market-cli`.
- Every installed Skill already passes the server-authoritative archive size,
  path, manifest, fingerprint, object-storage, and owner-library pipeline.
- Exact GitHub tree/blob Skill links and LobeHub Skill links are already
  partially recognized by Chat resource orchestration, but they do not yet
  share one complete Store and direct-install experience.
- Installing a Skill changes the owner's installed inventory only. Per-chat
  enablement remains an explicit conversation selection, with bounded Agent
  auto-selection during a run.

## Proposed Product Shape

- Keep one Skill page with `Installed` and `Skill Store` tabs.
- Make LobeHub Marketplace the browsable/searchable marketplace source, using
  the same interaction model as the MCP Marketplace: category rail, search,
  pagination, detail pane, install state, and isolated loading/error states.
- Keep OpenAI Curated available as a distinct trusted source/collection rather
  than silently replacing or mixing identities with LobeHub records.
- Add an explicit `Install from link` entry point in Skill Store and retain
  natural-language Chat installation for supported links.
- Always show source, resolved version/commit, package fingerprint, requested
  tools/permissions, and install status before or after installation as the
  existing immutable supply-chain contract requires.

## Requirements

- Add backend-owned LobeHub Skill list/search, category, detail, and install
  adapters using the existing Marketplace base URL, M2M token owner, bounded
  HTTP client, redacted errors, and retry policy.
- Validate LobeHub responses into Neo Chat-owned strict DTOs; never expose a
  raw upstream response contract to the frontend.
- Pin the exact version returned by Skill detail before download and reject
  mutable, missing, malformed, oversized, or identity-drifting packages.
- Route downloaded archives through the existing `skillsupply` validator,
  SHA-256 fingerprint, immutable object store, owner-private library, and
  conversation-selection boundaries.
- Preserve the existing OpenAI Curated catalog and installation path as a
  separate Store source.
- Add source-aware frontend state so LobeHub search/category/pagination does
  not corrupt OpenAI Curated state or the Installed tab.
- Support direct external Skill installation from the owner-approved source
  types decided below, using the same server validation and immutable install
  pipeline as Store installs.
- Keep Chat natural-language link installation deterministic: a supported
  direct Skill URL invokes the direct installer without depending on the model
  to invent a Store identifier.
- Do not execute an installed Skill during installation. Execution remains a
  later Agent-run concern governed by workspace access and conversation Skill
  selection.
- Add focused backend, frontend, URL-normalization, DTO, archive-security,
  ownership, failure-isolation, and regression tests.
- Update backend/frontend Skill supply-chain documentation and configuration
  notes when the API or operational behavior changes.

## Acceptance Criteria

- [x] Skill Store can browse and search LobeHub Skills with category filters,
      sorting/pagination, localized metadata, and actionable empty/error states.
- [x] Selecting a LobeHub item loads a strict detail view with source,
      identifier, exact version, author/license, requested permissions/tools,
      resources, validation state, and install state.
- [x] Installing a LobeHub Skill downloads one exact version through the Go
      backend and produces a normal owner-private installed Skill record.
- [x] The installed package fingerprint and file inventory are identical to
      the bytes validated before persistence; package drift fails closed.
- [x] OpenAI Curated remains independently browsable and installable.
- [x] A supported external Skill link can be installed from both the Store UI
      and Chat, then appears in `Installed` without being auto-enabled in every
      conversation.
- [x] Unsupported, ambiguous, private-without-credentials, malformed,
      oversized, duplicate, and path-traversal sources fail with actionable,
      source-safe errors and no partial installation.
- [x] Browser code never receives the LobeHub client secret or calls LobeHub's
      authenticated API directly.
- [x] MCP Marketplace behavior and existing installed Skill selections remain
      unchanged.
- [x] Focused frontend/backend tests, typecheck/lint/vet, and the appropriate
      cross-layer release gate pass.

## Owner Decision

The owner selected **A — Exact links** for the first release:

- Accept exact LobeHub Skill links.
- Accept exact GitHub tree/blob links that identify one Skill directory or its
  `SKILL.md`.
- Do not accept arbitrary webpages, arbitrary install commands, or local ZIP
  uploads in this slice.
- Keep the source allowlist and immutable package validation fail-closed; do
  not guess a Skill from an ambiguous repository or page.

## Out of Scope for the Initial Slice

- Publishing, claiming, rating, commenting, or managing a LobeHub identity.
- Browser-side LobeHub credentials or running the official Node CLI in the Go
  service container.
- Automatically enabling a newly installed Skill for all conversations.
- Executing package scripts, dependency installers, or arbitrary commands
  during installation.
- Treating MCP OAuth Skills and standalone instruction-package Skills as the
  same resource type.

## Technical Notes

- Official Marketplace routes: `GET /api/v1/skills`,
  `GET /api/v1/skills/categories`,
  `GET /api/v1/skills/{identifier}`, and
  `GET /api/v1/skills/{identifier}/download?version=...`.
- Existing reusable client:
  `mm-chat/backend/internal/mcpclient/marketplace_lobehub.go`.
- Existing supply chain:
  `mm-chat/backend/internal/skillsupply/`.
- Existing MCP Marketplace UX reference:
  `mm-chat/frontend/src/components/mcp/McpMarketplace.tsx`.
- Research and source mapping are recorded in
  `research/lobehub-skill-marketplace.md`.

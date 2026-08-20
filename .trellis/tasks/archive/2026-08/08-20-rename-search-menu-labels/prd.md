# Rename Search Menu Labels

## Goal

Make the composer Web search menu use the user-selected product wording and
persist a proportional verification rule for future localized changes.

## Confirmed Requirements

- Display the OpenAI built-in search option as `内置搜索` in Chinese instead
  of `OpenAI Web Search`.
- Display the server-provided external search option as `Tavily 搜索` instead
  of `Server 搜索`.
- Keep equivalent English and Japanese labels coherent.
- Change presentation only; do not alter search routing, capability checks, or
  Backend authority.
- For low-risk localized changes, run focused tests and changed-file checks
  instead of defaulting to the full repository/component suite.
- Build and deploy the Frontend because a build is necessary to publish the UI
  change, but do not run unrelated full test suites.
- Commit, archive, and record the work automatically without asking.

## Acceptance Criteria

- [x] Chinese OpenAI built-in search label is exactly `内置搜索`.
- [x] External server search label resolves to exactly `Tavily 搜索` in
      Chinese.
- [x] English/Japanese catalogs retain equivalent meanings and catalog parity.
- [x] Focused label/catalog tests and changed-file format/lint checks pass.
- [x] Live `18080` Frontend runs the new image and serves the updated compiled
      labels while Backend and unrelated containers remain unchanged.
- [x] The proportional testing rule is recorded in `AGENTS.md` and the
      Frontend quality spec.

## Technical Approach

Change `getSearchProviderLabel("default")` from `Server` to `Tavily`, update
the localized `searchModeOpenAIWeb` values to generic built-in-search wording,
and extend focused tests to assert the exact three-locale contract. Add a
testing-scope matrix to project guidance. Build a fresh immutable local
Frontend image, retain the current image/environment for rollback, and recreate
only the Frontend service.

## Out of Scope

- Search-provider selection, Tavily credentials, API behavior, model
  capability detection, or Backend changes.
- Full standalone, full Vitest, Backend, or RAG test suites.

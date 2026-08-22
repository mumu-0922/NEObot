# Persist Chat Model and Knowledge Detail on Refresh

## Goal

Keep the user's current chat model/provider selection and current Knowledge
collection detail view stable across a browser refresh in Server mode.

## Requirements

- Persist the complete chat model identity (`providerId:modelId`) as a
  browser-owned preference that remains writable in Server mode.
- Restore that persisted selection only after providers/models have hydrated,
  and retain the existing fallback when the saved model is no longer
  available.
- Continue supporting Local mode without replacing chat/session authority.
- Represent the selected Knowledge collection in the root-page URL while the
  Knowledge panel is open.
- Opening a collection, returning to the collection list, browser refresh, and
  browser back/forward navigation must keep the UI and URL synchronized.
- Treat collection IDs from the URL as untrusted. Do not fetch collection
  documents until the ID has been validated and matched to a collection
  returned by the server.
- Remove an invalid or deleted collection ID from the URL and return to the
  collection list without leaving a broken history entry.

## Acceptance Criteria

- [ ] Selecting a model under a non-default Provider and refreshing restores
      the same Provider and model.
- [ ] A missing or disabled saved model falls back to the existing preferred
      Provider/default-model resolution.
- [ ] Opening `知识库 > test` adds its collection ID to the URL; refreshing that
      URL returns to the `test` document-management page.
- [ ] Browser back/forward moves between the Knowledge list and collection
      detail consistently.
- [ ] An invalid, inaccessible, or deleted collection ID resolves to the
      Knowledge collection list and is removed from the URL.
- [ ] Focused persistence and URL-state tests, frontend lint, typecheck, and
      production build pass.

## Definition of Done

- Focused Vitest coverage proves the persisted Provider/model identity and
  Knowledge URL round-trip/normalization.
- Frontend format, lint, typecheck, tests, and build are green.
- The frontend is rebuilt and the running deployment is refreshed without
  changing server-owned runtime data.

## Technical Approach

- Extend `coreSettingsStore`, whose `getBrowserPreferenceStorage` authority is
  explicitly writable in Server mode, with a browser-owned selected chat model
  preference. Keep `chatStore.selectedModel` as the live composer state and
  synchronize both through the existing ChatApp selection path.
- Extend `panelUrlState` with a validated Knowledge collection query parameter.
  Let `ChatApp` own URL/history synchronization and pass the selected ID into
  `KnowledgeBase`/`ServerKnowledgeBase` as controlled state.
- Gate document loading on a collection object returned by
  `listCollections`, rather than requesting documents directly from an
  arbitrary query-string ID.

## Decision (ADR-lite)

**Context**: Server mode deliberately disables browser IndexedDB writes for
chat authority, but the selected composer model is a browser preference. The
Knowledge collection detail is navigation state, not durable domain data.

**Decision**: Store the full model identity in the existing browser-preference
store and store the active Knowledge collection in the URL.

**Consequences**: Model selection survives refresh without duplicating server
chat data; Knowledge detail becomes refreshable and browser-history aware.
Unavailable model/collection references are normalized to safe fallbacks.

## Out of Scope

- Per-conversation model defaults or backend schema changes.
- Persisting transient Knowledge search text, document checkbox selection, or
  modal state.
- Changing Provider configuration, Knowledge retrieval, parsing, or indexing.

## Technical Notes

- `chatStore` currently partializes `selectedModel`, but
  `getAppDbStorage()` is a no-op in Server mode by design.
- `coreSettingsStore` uses `getBrowserPreferenceStorage()` and is the existing
  home for browser-owned preferences that survive Server-mode refreshes.
- `ServerKnowledgeBase` currently initializes `selectedCollectionId` to
  `null`; `panelUrlState` currently serializes only the top-level Knowledge
  panel.
- Applicable specs: `.trellis/spec/frontend/state-management.md`,
  `.trellis/spec/frontend/hook-guidelines.md`, and
  `.trellis/spec/frontend/quality-guidelines.md`.

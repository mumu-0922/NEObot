# Remove built-in Browser (Playwright) MCP

## Goal

Completely retire the deployment-managed `Browser (Playwright)` MCP Server so
it no longer appears in Installed Tools or conversation selectors, while
preserving the separately installed private `Playwright MCP` Server.

## What I already know

- The built-in Server is the `manifest:playwright-browser-0.0.79` entry in
  `mm-chat/mcp/manifest.json`.
- The Installed page intentionally exposes delete only for private Servers, so
  a manifest Server cannot be removed through the browser UI.
- The current database has one conversation selection for the built-in Server
  and no Workspace selection. Removing only the manifest entry would leave a
  stale reference and make that Agent selection fail validation.
- The installed `Playwright MCP` is a private dynamic npm artifact with its own
  `runnerArtifactId`; it does not depend on the built-in manifest artifact.

## Requirements

- Remove `playwright-browser-0.0.79` from the checked-in MCP manifest.
- Add a forward database migration that removes the exact retired manifest ref
  from Conversation and Workspace selections and increments affected selection
  revisions.
- Remove exact credentials, OAuth state, and grants for the retired manifest
  ref while retaining historical Run snapshots and Tool-call audit rows.
- Keep the private `Playwright MCP` installation, the generic Playwright Runner
  capability, and all other MCP Servers unchanged.
- Replace the production-manifest Browser safety test with a regression test
  proving that the retired ID/name are absent.
- Remove current product/deployment documentation that claims the built-in
  Browser is shipped or selectable; preserve historical tracking records.
- Rebuild/restart the manifest-consuming Backend and MCP Runner services and
  verify the live MCP inventory no longer contains the built-in Server.

## Acceptance Criteria

- [x] `mm-chat/mcp/manifest.json` contains no `Browser (Playwright)` or
      `playwright-browser-0.0.79` entry.
- [x] Migration cleanup removes exact stale Conversation/Workspace selection
      rows and bumps revisions without deleting Tool-call history.
- [x] The private `Playwright MCP` remains ready with its 24 Tools.
- [x] Focused manifest, migration, MCP Backend, PostgreSQL 17 migration replay,
      and Compose configuration checks pass.
- [x] Deployed Installed Tools and conversation pickers no longer return the
      built-in Browser.

## Definition of Done

- Source, migration, tests, and current documentation agree on the retired
  built-in Server.
- Relevant services are recreated from the updated manifest and healthy.
- A focused conventional commit is created and the task is archived.

## Technical Approach

Use source retirement plus an exact forward data migration. Do not add a UI
special case: manifest inventory remains server-authoritative, and the item
disappears because its authoritative definition is gone. The migration fences
existing selections; immutable historical execution evidence remains intact.

## Decision (ADR-lite)

**Context:** Hiding the row would leave an executable shared definition, while
deleting only the definition would strand persisted selections.

**Decision:** Retire the manifest entry and clean only live references to its
exact source/ID in migration 108.

**Consequences:** Existing conversations silently lose that selected Server
and receive a revision bump. Re-adding the manifest later will not restore old
selection intent. The separate private Marketplace installation is unaffected.

## Out of Scope

- Uninstalling or changing the private `Playwright MCP` Server.
- Removing generic MCP Runner/Chromium support or the Playwright npm runtime.
- Rewriting historical tracking documents or deleting historical Tool calls.
- Redesigning Installed Tools deletion permissions.

## Technical Notes

- Manifest consumer: `MCP_MANIFEST_FILE=/etc/mm-chat/mcp-manifest.json` is
  mounted into Backend and MCP Runner by `compose.single-server.yml`.
- Persistence tables: `mcp_conversation_servers`, `mcp_workspace_servers`,
  selection parents, credentials, OAuth states, and grants.
- Current contract docs: `docs/contracts/chat-tool-loop.md` and
  `docs/deployment/mcp-runner.md`.

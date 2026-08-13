# Assistant Store

Neo Chat Assistants are server-owned, lightweight Conversation presets. They
contain display metadata and a bounded System Prompt; they are not executable
Agents and never install Models, credentials, MCP Servers, plugins, Knowledge
Bases, or permissions.

## Authority

- `assistant_library_entries` owns each user's installed and custom Assistants.
- `assistant_market_admissions` owns the administrator's explicit LobeHub
  admission decision and the exact reviewed snapshot.
- A Store install is accepted only when the requested identifier and canonical
  content fingerprint match the currently admitted snapshot.
- Custom edits and Store updates use an expected revision. Stale writers receive
  `ASSISTANT_REVISION_CONFLICT` and cannot overwrite a newer device's value.
- Copying a Store snapshot into a custom Assistant also binds the source
  revision, so a stale device cannot silently copy a different upstream
  snapshot than the one it reviewed.
- Structured `required_tools` declarations are normalized display metadata and
  participate in the canonical fingerprint. They are visible in Store and My
  Assistants and link to Tools, but never install, select, enable, or authorize
  a Tool.
- Starting a chat copies the installed System Prompt into the Conversation.
  Later update, uninstall, or deletion cannot rewrite existing Conversations.

## Marketplace boundary

The Backend prefers LobeHub's authenticated Market API through the same bounded
M2M transport and token/cache authority used by MCP Marketplace. It reads
`/api/v1/agents` and `/api/v1/agents/detail/{id}` with response-size, timeout,
page, schema, and normalization bounds. The public React Router `.data` route
remains the first fallback, followed by the legacy `@lobehub/agents-index`
registry. The category rail follows LobeHub's Assistant UI allowlist and does
not expose aggregate-only `engineering` or `uncategorized` buckets. The global
market count is display-only and remains separate from the browseable result
count.

Ordinary users never browse arbitrary upstream results. Their search and detail
APIs return only `admitted` server snapshots, and install rechecks that same
authority. Administrators may review a live detail and admit or reject its
exact fingerprint.

## Rollback

Application rollback can leave migration `082` in place safely; old versions
ignore the additive tables. Schema rollback removes Assistant library and
admission rows and is therefore destructive. Back up PostgreSQL before running
the down migration. Use `scripts/verify-assistant-store-postgres17.sh` for the
disposable PostgreSQL 17 `081 -> 082 -> 081 -> 082` replay and repository
ownership/CAS drill.

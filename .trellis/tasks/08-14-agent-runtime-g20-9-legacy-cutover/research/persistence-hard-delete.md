# G20.9 persistence hard-delete design

## Stores and exact deletion set

`STORAGE_VERSION` is currently `6` and is shared by Settings, Chat, core
preferences and Memory stores. G20.9 advances it once. Only these retired values
are deleted:

- settings state: `installedSkills`, `customSkills`, `activeSkillIds`,
  `skillAutoSelect`, `skillCatalogs`, `skillCatalogTimestamps`,
  `skillDefinitions`, `skillDefinitionTimestamps`;
- every local Session `config.activeSkills`;
- every local Workspace `activeSkills`;
- server Conversation JSONB metadata key `activeSkills` through the explicit
  PostgreSQL cutover operation.

Delete settings fields both when the raw persistence record is the state object
and when it is a Zustand `{ state, version }` envelope. This covers old direct
fixtures/imports plus current IndexedDB records.

## Preservation set

The migration must preserve all unrelated keys and values without normalization
side effects: Assistant references/cache retirement state, MCP configuration,
Chat messages/tree, attachments/files, Knowledge collection selections, Memory,
providers/BYOK shells, voice, search, theme and core preferences. In particular,
`selectedKnowledgeCollectionIds` is adjacent to `activeSkills` and must survive.

## Layered enforcement

1. Pure strip functions clone only touched objects and are idempotent.
2. The explicit raw-browser migration stages localStorage/IndexedDB settings and
   Chat main keys, writes only changed records, and writes its marker last.
3. Zustand `migrate` repeats the deletion defensively after the version bump.
4. Settings/Chat `partialize` builds persisted values without any retired field,
   preventing resurrection after reload.
5. Session/Workspace normalizers destructure and discard stale selection values,
   so imports and programmatic writes cannot recreate them.
6. Runtime types and store actions no longer expose the fields.

The raw migration should compensate already-written records if a later write
fails, and it must never set completion on invalid JSON or a failed write.
Repeated execution after partial failure is safe because stripping is
idempotent.

## Backup and destructive sequencing

G20.8 already exposes an explicit raw local settings backup and deterministic
content-free inventory/dry-run. Production deployment order is:

```text
freeze legacy mutation -> export raw local backup -> match inventory fingerprint
-> record PostgreSQL backup + activeSkills row count -> deploy G20.9 source
-> run browser migration + explicit PostgreSQL delete transaction
-> reload/restart and prove zero resurrection
```

The G20.9 source must not claim the backup happened. Deployment remains held
until an operator/user has the G20.8 artifact and the fingerprint/count gate.
Rollback during the declared window restores both the old image and exact
backup. After accepting package installs/new Runtime writes, prefer forward
repair; never synthesize old definitions from message history.

## Required negative proof

- Top-level and nested settings cases both lose exactly eight keys.
- Sessions/Workspaces lose only `activeSkills`, including empty/invalid arrays.
- `partialize` cannot serialize any retired field after migration and reload.
- Marker is written only after all browser records succeed.
- PostgreSQL dry-run reports count only; apply requires explicit confirmation and
  expected count, removes only the named JSONB key, and returns zero remaining.
- Reload/restart/import do not resurrect authority.

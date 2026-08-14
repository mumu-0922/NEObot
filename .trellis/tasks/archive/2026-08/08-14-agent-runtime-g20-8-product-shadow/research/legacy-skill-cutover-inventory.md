# G20.8 legacy text-Skill inventory

## Current authority

- Legacy pure-text Skills live in the frontend `settingsStore` fields
  `installedSkills`, `customSkills`, `activeSkillIds`, catalog/definition caches
  and related IndexedDB persistence.
- Server package Skills live in PostgreSQL/object storage and are a separate
  identity/fingerprint domain.
- The approved migration choice is deletion in G20.9 followed by fresh Store or
  manual installation; G20.8 must not auto-convert text to executable packages.

## Recommended inventory

- Generate a local, content-free cutover manifest containing schema version,
  counts, normalized IDs, per-record fingerprints, active references, cache
  counts and invalid/orphan counts. Never upload Skill bodies by default.
- Offer an explicit local JSON backup export before the destructive drill. The
  export may contain legacy bodies because it is user-requested local backup,
  but it must not enter backend logs, telemetry or committed fixtures.
- Add a dry-run reset plan that reports exactly which IndexedDB/store keys would
  be removed and which unrelated Assistant/MCP/Chat settings remain.
- G20.8 verifies inventory and dry-run only. G20.9 owns confirmation, deletion,
  restart/reload proof and removal of the legacy invocation path.

## Negative boundaries

- Do not reinterpret legacy Skill IDs as package fingerprints.
- Do not install a package merely because a legacy title/name matches.
- Do not delete Assistant presets, MCP selection, Conversation data or user
  files as part of Skill cleanup.
- Do not persist raw legacy bodies in server migration tables or diagnostics.

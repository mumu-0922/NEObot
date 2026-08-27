# Technical design

## Ownership

- `internal/skillsupply`: GitHub catalog discovery, immutable Skill source
  resolution, validation, ingest, installation, and HTTP DTOs.
- `internal/resourceorchestrator`: supported GitHub Skill link recognition and
  deterministic chat installation routing only; it delegates all writes.
- frontend `skillStore` client: strict catalog DTOs and server adapter.
- `components/skills/SkillStore`: OpenAI curated catalog presentation and
  existing Installed management.
- existing conversation resource picker and local Skill runtime remain
  unchanged unless tests expose a regression.

## API sketch

```text
GET  /v1/skills/catalog
GET  /v1/skills/catalog/items/{name}
POST /v1/skills/catalog/items/{name}/install
     { resolvedCommit, packageFingerprint }
```

The list endpoint returns bounded entries from the fixed
`openai/skills/skills/.curated` source. Detail returns validated metadata,
resolved commit, and package fingerprint. Install accepts the displayed
commit/fingerprint and rejects drift before delegating to the existing
owner-private ingest/install path.

## Caching

Cache only upstream catalog listing/ref resolution for a short bounded TTL.
Never cache per-user installation state as catalog authority. Catalog failure
must not affect Library reads or runtime materialization.

## Compatibility

- Keep admitted Store endpoints for internal/resource-orchestration backward
  compatibility until a later explicit cleanup.
- Keep AIHero direct-link support.
- Add GitHub tree/blob support to the same direct adapter and link scanner.
- Do not change database schema unless exact source identity cannot be projected
  from existing candidate/install joins; prefer a joined projection over a new
  persistence authority.

## Verification map

- Source/parser unit tests: GitHub Contents, commit pinning, tree/blob parsing,
  archive selection, invalid URL/path/ref, size/bounds.
- Service/handler tests: detail, fingerprint-bound install, duplicate,
  unavailable upstream, strict HTTP.
- Resource orchestrator tests: explicit intent, one link, zero Provider call,
  malformed/multiple links, AIHero compatibility.
- Frontend tests: strict DTOs, endpoint calls, Store composition, search/detail,
  install refresh, Installed operation, hidden internal review language.
- Full affected-component gates and standalone cross-layer verification.

# LobeHub Skill Marketplace Research

## Official Behavior

- `https://lobehub.com/skills` redirects to
  `https://market.lobehub.com/s/skills`, which documents the official Skill
  Marketplace workflow.
- Official CLI package inspected: `@lobehub/market-cli@0.0.41`.
- Official SDK package inspected: `@lobehub/market-sdk@0.40.1`.
- Anonymous API access returns `401 Missing bearer token`; Neo Chat must use
  its server-owned Marketplace M2M token.
- The CLI requires registration before Marketplace operations, but Neo Chat
  already owns equivalent server credentials for MCP Marketplace.
- The CLI downloads a ZIP containing root `SKILL.md` plus bundled resources.
  Neo Chat must consume the protocol, not invoke the CLI or copy its unsafe
  direct extraction behavior.

## Read API Contract

### List and search

`GET /api/v1/skills`

Supported query fields include:

- `q`, `category`, `isFeatured`, `isOfficial`, `locale`
- `page`, `pageSize`
- `sort`: `commentCount`, `createdAt`, `forks`, `installCount`, `name`,
  `ratingAverage`, `recommended`, `relevance`, `stars`, `updatedAt`, `watchers`
- `order`: `asc` or `desc`

The result contains `items`, `currentPage`, `pageSize`, `totalCount`, and
`totalPages`. Each item exposes identifier, canonical name, description,
version, category, author, validation/official/featured flags, install/rating
counts, tags, timestamps, optional logo/icon/homepage/license, and GitHub
metadata.

### Categories

`GET /api/v1/skills/categories?locale=...&q=...`

Returns category/count pairs. Search filtering can be carried into the category
counts.

### Detail

`GET /api/v1/skills/{identifier}?locale=...&version=...`

The detail adds the complete `SKILL.md` body, parsed manifest, overview,
resource path/hash/size map, author/license/repository metadata, exact current
version/version number, version history, and validation timestamp.

### Download

`GET /api/v1/skills/{identifier}/download?version={exactSemver}`

The official client defaults to latest when version is omitted. Neo Chat must
first resolve detail and then always provide the exact returned version to
avoid a list/detail/download race.

## Existing Neo Chat Reuse

- `mcpclient.LobeHubMarketplace` already owns token acquisition, authenticated
  GET/download, response size limits, URL validation, error redaction, and the
  existing MCP Marketplace operations.
- Its `FetchSkillPackage` method already downloads an exact semver Skill ZIP
  from the official download route with a 32 MiB hard ceiling.
- `cmd/api/main.go` already injects the same LobeHub client into `skillsupply`,
  so a second credential store or Node sidecar is unnecessary.
- `skillsupply` already validates root `SKILL.md`, strict frontmatter/runtime
  manifests, unique/safe paths, archive size/file count, content hashes,
  immutable fingerprint, source identity, object persistence, owner-private
  installation, and conversation selections.
- Current Store catalog is separately fixed to OpenAI Curated. It should remain
  a distinct source, not be represented as synthetic LobeHub records.
- Current direct-link source supports exact GitHub tree/blob coordinates and a
  legacy AIHero adapter. LobeHub URL parsing exists in resource orchestration,
  but currently falls back to Store search instead of an exact LobeHub detail
  and pinned download.

## Architecture Decision

Use one Neo Chat Skill Store API façade with explicit source identity:

1. Frontend calls Neo Chat only.
2. Backend source adapter calls LobeHub with the existing M2M client.
3. Upstream DTOs are strictly decoded and normalized into Neo Chat DTOs.
4. Detail resolves an exact version.
5. Install downloads that exact version and feeds the unchanged immutable
   validation/persistence pipeline.
6. Installed inventory and per-conversation selection remain source-agnostic.

This keeps marketplace discovery replaceable while keeping trust,
installation, and runtime policy centralized.

## Threat Boundary

- Treat every marketplace or external package as untrusted input even when
  LobeHub marks it official or validated.
- Never execute package content during installation.
- Allowlist source host/scheme/path shapes and reject redirects outside the
  intended host policy.
- Bound list/detail JSON, archive bytes, decompressed bytes, file count, file
  names, manifest size, and HTTP time.
- Do not log bearer tokens, upstream bodies, package contents, or private URLs.
- Fail installation atomically; no library row should reference an incomplete
  object.
- Record source identifier, exact version/commit, package fingerprint, and
  installer owner for auditability.

## Primary Sources

- LobeHub Skills Marketplace documentation:
  `https://market.lobehub.com/s/skills`
- Installation reference:
  `https://market.lobehub.com/s/skills/references/skills-install`
- Official CLI repository metadata:
  `https://github.com/lobehub/lobehub-market`
- Official SDK repository metadata:
  `https://github.com/lobehub-biz/lobehub-market`

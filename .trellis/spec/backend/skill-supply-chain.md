# Skill Supply Chain

## 1. Scope / Trigger

Apply when changing `/v1/skills/*`, public Skill discovery, GitHub source
parsing, package validation/ingestion, Library management, Conversation Skill
selection, or Skill runtime materialization.

The public product model follows Codex discovery semantics but not its local
filesystem trust model: Neo Chat keeps the backend-mediated LobeHub Marketplace
and fixed curated GitHub directory as separate discovery sources, accepts only
exact LobeHub/GitHub Skill links, and retains server-side immutable source,
package, owner, and runtime authority.

## 2. Signatures

```text
GET  /v1/skills/catalog
GET  /v1/skills/catalog/items/{name}
POST /v1/skills/catalog/items/{name}/install
     { resolvedCommit, packageFingerprint }

GET  /v1/skills/marketplace?q=&category=&page=&pageSize=
GET  /v1/skills/marketplace/items/{identifier}?version={exactSemver}
POST /v1/skills/marketplace/items/{identifier}/install
     { version: exactSemver }
POST /v1/skills/direct/install
     { url: exactLobeHubOrGitHubSkillURL }

GET    /v1/skills/library
DELETE /v1/skills/library/{installationId}?revision={revision}

GET /v1/skills/conversations/{conversationId}/selection
PUT /v1/skills/conversations/{conversationId}/selection
    { revision, installationIds }
```

```go
func (*Service) ListCatalog(context.Context) (CatalogResult, error)
func (*Service) GetCatalogSkill(context.Context, name string) (CatalogSkill, error)
func (*Service) InstallCatalogSkill(
    context.Context, userID, name string, input CatalogInstallInput,
) (Installation, error)
func (*Service) InstallDirectSkillLink(
    context.Context, userID, rawURL, expectedName string,
) (Installation, error)
func ParseGitHubSkillURL(raw string) (GitHubSkillCoordinate, error)
func ParseLobeHubSkillURL(raw string) (identifier string, err error)
func (DirectSkillLinkSource) fetchGitHubSubtree(
    context.Context, rawURL, expectedName string,
) (ArchiveSource, error)
func (*Service) SearchMarketplace(
    context.Context, MarketplaceSearchInput,
) (MarketplaceSearchResult, error)
func (*Service) GetMarketplaceSkill(
    context.Context, userID, identifier, version string,
) (MarketplaceSkillDetail, error)
func (*Service) InstallMarketplaceSkill(
    context.Context, userID, identifier, exactVersion string,
) (Installation, error)
```

## 3. Contracts

- The default public catalog is fixed to repository `openai/skills`, ref
  `main`, path `skills/.curated`. The Backend alone calls GitHub; the browser
  consumes strict sanitized DTOs.
- Catalog list uses the GitHub Contents API, returns at most 256 directories,
  and caches the bounded list for five minutes. It never returns package bytes,
  object keys, local paths, credentials, raw upstream errors, admission IDs,
  SBOM data, or fingerprints.
- Catalog detail resolves `main` to one lowercase 40-character commit, fetches
  the codeload ZIP, selects `skills/.curated/<name>`, and calls
  `ValidateArchive`. Its installable projection includes the exact commit and
  canonical `sha256:` package fingerprint.
- LobeHub list/category uses the existing backend-owned Marketplace M2M client.
  Exact detail uses a separate bounded public GET because LobeHub rejects M2M
  bearer tokens on that route. The exact detail's internal-only
  `manifest.sourceUrl` is package transport; browser code receives only the
  normalized LobeHub page URL and never sees that GitHub coordinate, a bearer
  token, raw upstream body, or package archive.
- LobeHub Skill category responses accept at most 512 validated raw rows, then
  project only the product-owned ordered allowlist of 21 canonical category
  IDs with their live counts. Community tags remain discoverable through All
  and search but never enter the navigation DTO. Malformed or unexpectedly
  unbounded auxiliary responses fail open for the independently valid item
  list.
- An explicit Marketplace category must be one of those 21 canonical IDs and
  is rejected before M2M transport otherwise. Valid filters are forwarded
  unchanged so category results remain upstream-authoritative rather than a
  client-side filter over the current page.
- Locale and sort values cross the authenticated Marketplace boundary only
  through explicit allowlists; unknown values fail before upstream I/O.
- Marketplace install must re-read detail for the requested exact SemVer,
  require the response identity/version to match, parse the exact GitHub
  `tree`/`blob` `manifest.sourceUrl`, and require its terminal directory to
  equal the manifest name before GitHub I/O. `latest`, tags, ranges, and
  list-only version data are never install authority.
- Marketplace GitHub transport resolves a mutable ref to a lowercase
  40-character commit, enumerates only the declared directory with bounded
  GitHub Contents responses, accepts only regular files/directories, verifies
  every raw file against its declared Git blob SHA, and builds one bounded
  canonical ZIP. It never downloads the enclosing codeload repository, follows
  redirects, accepts symlinks/submodules, or executes source content.
- The validated bytes are rebound to
  `lobehub:<identifier>@<exactSemver>` before ingestion. They then enter the
  unchanged `ValidateArchive`, object, fingerprint, owner-private Candidate,
  and Library path, preserving installed lookup and idempotency.
- LobeHub `identifier` is source identity and may differ from the root
  `SKILL.md` manifest `name`. Detail supplies the validated manifest name to
  `LobeHubSource.FetchAs`; source references remain
  `lobehub:<identifier>@<exactSemver>`.
- Instruction-only Codex Skills may omit `metadata.version` and
  `allowed-tools`. Validation retains the deterministic content-fingerprint
  version fallback, and every public `allowedTools` projection serializes an
  empty collection as JSON `[]`, never `null`; strict frontend DTO validation
  must not be weakened to absorb a producer-side nil-slice bug.
- `SKILL.md` `metadata` must be a bounded mapping with unique string keys.
  Bounded top-level string values such as `version` are projected into Neo
  Chat's `map[string]string`; nested mappings and sequences used by ecosystems
  such as OpenClaw are structurally validated, bounded to 16 levels and 4,096
  nodes, then kept opaque. YAML aliases, duplicate nested keys, typed
  top-level scalars, and over-limit structures are rejected. Nested
  `requires`, `install`, `bins`, secrets, tools, or permissions never become
  runtime authority and are never executed.
- Catalog install requires the displayed exact commit and fingerprint. It
  re-fetches that immutable commit, revalidates the selected directory, rejects
  package drift, then reuses canonical ZIP, SBOM, content-addressed storage,
  owner-private candidate, and Library installation authority.
- Exact direct links must use HTTPS `github.com` tree URLs identifying one
  Skill directory, or blob URLs ending in that directory's `SKILL.md`.
  Repository roots, ambiguous paths, multi-segment refs, percent encoding,
  traversal, query, fragment, userinfo, and non-default ports are rejected.
  Mutable single-segment refs are resolved to an exact commit before fetch.
  Because codeload always returns the whole repository ZIP, every explicitly
  selected subdirectory must additionally prove that
  `<archive-root>/<subdirectory>/SKILL.md` exists in the pinned archive before
  ingestion. Do not infer existence from the codeload HTTP status.
- Exact LobeHub links allow only `https://{lobehub.com|www.lobehub.com|market.lobehub.com}/skills/{identifier}`
  with no query, fragment, userinfo, non-default port, encoding ambiguity, or
  extra segment. Chat detects exactly one supported link and invokes the same
  owner-private exact-version installer without a Provider turn or Store
  search.
- The legacy AIHero adapter remains compatible but is not the catalog. It
  parses one restricted repository/Skill coordinate as data and executes no
  displayed command.
- Discovery and installation execute no npm, Git, Shell, hooks, tests, Skill
  scripts, or package entrypoints. Installed content remains untrusted until
  the ordinary runtime selection, revalidation, Tool-policy, and Workspace
  checks pass.
- Catalog and direct installs create owner-private `validated` candidates.
  They require no administrator review, remain absent from legacy `/store`,
  and are installable only by the same owner.
- Installation changes inventory only. Durable enablement remains a
  revision-CAS Conversation selection; `agent_auto` may activate at most the
  existing bounded relevant installed set for one frozen Run and never writes
  selection state.
- Catalog failure is isolated: Library, uninstall, Conversation selection, and
  runtime materialization remain available.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| catalog query string, unknown JSON field, invalid name | bounded `4xx`; no source fetch/install |
| GitHub list/ref/archive unavailable or malformed | `502 SKILL_SOURCE_UNAVAILABLE`; no raw upstream body |
| detail source has invalid archive/frontmatter/path/symlink | typed validation failure; no installation |
| `metadata` has bounded nested community hints | accept as inert package content; project only safe top-level strings |
| `metadata` root/key/alias/depth/node bounds are invalid | `INVALID_SKILL_PACKAGE`; zero Candidate/object/Library mutation |
| selected exact GitHub directory has no root `SKILL.md` in the pinned ZIP | invalid direct source; zero ingestion/object/database mutation |
| valid Skill omits `allowed-tools` | detail/install/Library DTOs contain `allowedTools: []` |
| install commit is not 40 lowercase hex or fingerprint is malformed | `400`; no fetch/install |
| install revalidation fingerprint differs | `409 SKILL_PACKAGE_CHANGED`; reload detail |
| exact installation already exists | return/reconcile the existing owner installation |
| GitHub repository root, encoded/traversal/query/fragment URL | reject direct route; zero mutation/Provider calls |
| direct source belongs to another owner | deny installation even with candidate/fingerprint knowledge |
| catalog unavailable after prior installs | Installed/selection/runtime paths continue independently |
| unknown Marketplace query or malformed identifier/version | `400`; zero M2M/package/database mutation |
| explicit category outside the 21-ID allowlist | `400 INVALID_SKILL_MARKETPLACE_QUERY`; zero M2M calls |
| category source has more than 512 rows | omit categories but retain the independently valid item list |
| Marketplace list/category/detail unavailable or malformed | `502 SKILL_SOURCE_UNAVAILABLE`; no upstream body leak |
| detail identity/version changes before install | `409 SKILL_PACKAGE_CHANGED`; zero package persistence |
| Marketplace detail source missing, malformed, or terminal-name mismatched | reject before GitHub I/O and persistence |
| Marketplace target subtree missing root `SKILL.md`, exceeds bounds, contains a symlink/submodule, or has Git blob drift | typed source/archive failure; zero Candidate/object/Library mutation |
| exact LobeHub/GitHub link malformed or ambiguous | `400 INVALID_SKILL_PACKAGE`; zero Provider/source execution |

## 5. Good / Base / Bad Cases

- **Good:** list curated names, validate one exact detail, install its exact
  commit/fingerprint, display it in Library, and let the user select it for one
  Conversation.
- **Good:** search LobeHub, inspect `owner-demo@1.2.3`, re-resolve that exact
  version, pin its declared GitHub Skill directory, validate its root manifest,
  rebind the LobeHub source identity, and install it only for the caller.
- **Good:** select `productivity-tasks`, forward that exact canonical ID, and
  return only the ordered curated category/count projection alongside results.
- **Good:** install an OpenClaw-authored Skill whose namespaced nested metadata
  declares a binary installer; retain the Skill instructions while leaving the
  installer and requirement declarations inert.
- **Base:** GitHub is temporarily unavailable; the catalog shows bounded retry
  state while Installed Skills remain manageable and usable.
- **Base:** a long-tail Skill has only a community tag; it remains visible in
  All/search without adding that tag to the navigation rail.
- **Bad:** scrape a third-party marketplace in the browser, install a whole
  repository, trust a mutable branch at install time, expose raw fingerprints
  as product language, or execute an upstream install command.
- **Bad:** treat list `version` as sufficient install authority, download
  latest without detail revalidation, or require Marketplace identifier to
  equal manifest name.
- **Bad:** use LobeHub's currently unauthorized `/download` route or download a
  97 MiB repository ZIP to install a 3 KiB nested Skill.

## 6. Tests Required

- Catalog list bounds/filter/sort/cache and exact fixed-source projection.
- Detail branch-to-commit resolution, selected-subdirectory validation, and
  archive/path/symlink rejection.
- Detail and installation fixtures must include an official-style
  instruction-only Skill with no version and no `allowed-tools`; assert a
  deterministic non-empty version and byte-level `"allowedTools":[]` JSON.
- Catalog HTTP list/detail/install strictness, stale fingerprint conflict,
  idempotent retry, Library presence, and legacy Store exclusion.
- Direct GitHub tree/blob acceptance plus root, encoded traversal, query,
  fragment, mismatch, and ambiguous input rejection.
- LobeHub transport tests must prove list/category retain M2M authorization,
  exact detail omits Authorization, Marketplace installation makes zero legacy
  package-download calls, and generic/plugin/download/encoded-path requests
  fail before HTTP.
- Marketplace list/detail/install tests must assert pagination/category DTOs,
  exact version, identifier/manifest-name separation, owner-private Candidate,
  installed status, idempotent reconciliation, and no Store publication.
- Marketplace subtree tests must assert mutable-ref pinning, exact-directory
  enumeration, nested regular-file materialization, zero codeload traffic,
  Git blob verification, symlink rejection before blob fetch, and zero mutation
  on malformed source or content drift.
- Parser and Marketplace integration tests must include OpenClaw-shaped nested
  metadata, assert that it installs through the pinned exact subtree, assert
  that only a top-level string `version` is projected, and reject duplicate,
  non-mapping, aliased, typed-scalar, over-depth, and over-entry metadata.
- Category tests must feed the live-scale raw taxonomy, assert the exact
  ordered 21-ID projection, prove `_meta` is removed, and prove a non-curated
  query is rejected before the Marketplace fetcher records a request.
- Deterministic Chat link installation must prove zero Provider and zero Store
  search calls.
- Frontend strict Zod catalog identity, list/detail/install/uninstall routes,
  URL round-trip, responsive/focus/error behavior, and absence of internal
  supply-chain fields in the normal surface.
- Run Go vet/tests, frontend format/lint/typecheck/Vitest/build, Skill supply
  verification, Compose render, and the full standalone gate.

## 7. Wrong vs Correct

```text
Wrong: browser -> arbitrary marketplace HTML -> mutable URL -> npm install
Correct: Backend source adapter -> exact version/commit -> ValidateArchive -> owner Library

Wrong: list says 1.2.3 -> download latest -> accept whatever arrives
Correct: exact detail 1.2.3 -> pin declared GitHub directory -> rebind exact LobeHub identity

Wrong: nested Marketplace Skill -> download the enclosing repository ZIP
Correct: pinned exact directory -> bounded file list -> verified Git blobs -> ValidateArchive

Wrong: GitHub repository root -> guess a nested Skill
Correct: exact tree directory or blob/SKILL.md -> resolve ref -> exact directory

Wrong: codeload returned 200 -> assume the requested subdirectory exists
Correct: pinned ZIP -> require selected-root SKILL.md -> ValidateArchive

Wrong: nil Go slice -> JSON null -> loosen the frontend Zod schema
Correct: normalize the Backend collection -> JSON [] -> retain strict Zod

Wrong: nested metadata contains install hints -> reject the whole community Skill or execute the hints
Correct: validate bounded structure -> keep it opaque -> grant no runtime authority

Wrong: install -> silently enable for every Conversation
Correct: install inventory -> user selection or bounded run-only agent_auto

Wrong: expose every upstream tag -> unstable mixed-case navigation taxonomy
Correct: project 21 canonical IDs -> keep All/search for long-tail discovery
```

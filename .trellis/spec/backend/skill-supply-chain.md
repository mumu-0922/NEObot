# Skill Supply Chain

## 1. Scope / Trigger

Apply when changing `/v1/skills/*`, public Skill discovery, GitHub source
parsing, package validation/ingestion, Library management, Conversation Skill
selection, or Skill runtime materialization.

The public product model follows Codex discovery semantics but not its local
filesystem trust model: Neo Chat lists a fixed curated GitHub directory and
accepts exact GitHub Skill paths while retaining server-side immutable source,
package, owner, and runtime authority.

## 2. Signatures

```text
GET  /v1/skills/catalog
GET  /v1/skills/catalog/items/{name}
POST /v1/skills/catalog/items/{name}/install
     { resolvedCommit, packageFingerprint }

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
- Instruction-only Codex Skills may omit `metadata.version` and
  `allowed-tools`. Validation retains the deterministic content-fingerprint
  version fallback, and every public `allowedTools` projection serializes an
  empty collection as JSON `[]`, never `null`; strict frontend DTO validation
  must not be weakened to absorb a producer-side nil-slice bug.
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
| selected exact GitHub directory has no root `SKILL.md` in the pinned ZIP | invalid direct source; zero ingestion/object/database mutation |
| valid Skill omits `allowed-tools` | detail/install/Library DTOs contain `allowedTools: []` |
| install commit is not 40 lowercase hex or fingerprint is malformed | `400`; no fetch/install |
| install revalidation fingerprint differs | `409 SKILL_PACKAGE_CHANGED`; reload detail |
| exact installation already exists | return/reconcile the existing owner installation |
| GitHub repository root, encoded/traversal/query/fragment URL | reject direct route; zero mutation/Provider calls |
| direct source belongs to another owner | deny installation even with candidate/fingerprint knowledge |
| catalog unavailable after prior installs | Installed/selection/runtime paths continue independently |

## 5. Good / Base / Bad Cases

- **Good:** list curated names, validate one exact detail, install its exact
  commit/fingerprint, display it in Library, and let the user select it for one
  Conversation.
- **Base:** GitHub is temporarily unavailable; the catalog shows bounded retry
  state while Installed Skills remain manageable and usable.
- **Bad:** scrape a third-party marketplace in the browser, install a whole
  repository, trust a mutable branch at install time, expose raw fingerprints
  as product language, or execute an upstream install command.

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
Correct: Backend fixed catalog -> exact commit -> ValidateArchive -> owner Library

Wrong: GitHub repository root -> guess a nested Skill
Correct: exact tree directory or blob/SKILL.md -> resolve ref -> exact directory

Wrong: codeload returned 200 -> assume the requested subdirectory exists
Correct: pinned ZIP -> require selected-root SKILL.md -> ValidateArchive

Wrong: nil Go slice -> JSON null -> loosen the frontend Zod schema
Correct: normalize the Backend collection -> JSON [] -> retain strict Zod

Wrong: install -> silently enable for every Conversation
Correct: install inventory -> user selection or bounded run-only agent_auto
```

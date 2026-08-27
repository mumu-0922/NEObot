# Codex-style Skill Installer

## Context

Neo Chat already owns a secure Skill supply chain: immutable source pinning,
archive validation, canonical package storage, SBOM generation, per-user
installation, per-conversation selection, runtime materialization, and bounded
Agent auto-activation. The current user-facing Store, however, exposes only
administrator-admitted candidates. That does not match the interaction the
product now promises: browse a useful public Skill list, paste a GitHub Skill
link into Chat, install it directly, and use any installed Skill from the
conversation picker or Agent auto-selection.

OpenAI Codex provides the reference behavior. Its default catalog is the
curated area of `openai/skills`; an install resolves a repository path, checks
for `SKILL.md`, copies a complete Skill directory into the local library, and
later activates the Skill explicitly or by description matching. Neo Chat must
adopt that product model without copying Codex's local-only trust boundary or
bypassing Neo Chat's server-side validation and ownership controls.

## Goal

Replace the public Skill Store discovery path with a Codex-style GitHub catalog
and direct GitHub Skill installation while retaining Neo Chat's existing secure
package authority, installed-library management, conversation selection, and
Agent runtime activation.

## User stories

1. As a user, I can open Skill Store and browse/search the OpenAI curated
   `openai/skills` list without an administrator first admitting every entry.
2. As a user, I can inspect a curated Skill's description and GitHub source,
   install it, and then see it under Installed.
3. As a user, I can paste one supported GitHub Skill URL into an Agent chat with
   install intent and receive a deterministic success or failure without the
   Provider improvising shell/npm installation.
4. As a user, I can select installed Skills independently for each conversation.
5. As an Agent user, relevant installed Skills may still be activated for one
   run by the existing bounded `agent_auto` policy.
6. As an operator, an unavailable or rate-limited GitHub catalog does not break
   already-installed Skills or expose secrets/internal package paths.

## Functional requirements

### Catalog

- Add an authenticated, `no-store`, server-owned catalog API backed by the
  fixed default source `openai/skills`, `skills/.curated`, ref `main`.
- Catalog results are bounded and contain only sanitized display metadata and
  stable source coordinates; no archive bodies, credentials, local paths, or
  raw upstream errors.
- Catalog detail resolves the mutable ref to an exact 40-character commit,
  downloads the exact repository archive through the existing GitHub source,
  extracts the selected Skill directory, and validates it through the existing
  package validator before returning installable detail.
- Catalog failures are typed and bounded. Existing installed/library APIs stay
  available.

### Installation

- Catalog installation binds the displayed source coordinate, exact commit,
  and validated package fingerprint.
- Installation reuses the current ingest path: canonical archive, SBOM,
  content-addressed objects, private owner scope, repository installation, and
  idempotent read-back behavior.
- No npm, Git, shell command, repository hook, Skill script, or package entry
  point is executed during discovery or installation.
- A duplicate exact installation returns/reconciles the existing installation.
- Installed Skills remain managed by `/v1/skills/library` and existing
  revision-bound uninstall behavior.

### GitHub link installation from Chat

- Accept only canonical HTTPS GitHub Skill links shaped as
  `/owner/repository/tree/<ref>/<skill-path>` or
  `/owner/repository/blob/<ref>/<skill-path>/SKILL.md`.
- Reject repository-root links without an unambiguous Skill directory, as well
  as query, fragment, userinfo, non-default port, traversal, controls,
  unsupported host/scheme, oversized input, or multiple links.
- Normalize a blob link to its parent Skill directory, resolve the requested
  ref to an exact commit, and reuse the same bounded GitHub fetch/validation/
  ingest path as catalog installation.
- Preserve the existing AIHero direct-link adapter for backward compatibility.
- Extend deterministic resource-link recognition so an explicit one-link
  install turn reaches the server-owned installer before any Provider call.

### UI

- Keep the existing `Installed | Skill Store` layout and installed management.
- Replace the admitted-candidate Store content with the OpenAI curated catalog.
- Search locally within the bounded catalog response, show a clear source badge,
  and load validated detail on selection.
- Present human-facing name, description, source repository/path, compatibility,
  and install state; do not expose internal admission IDs, reviewer state, SBOM
  internals, or raw fingerprints as the primary product language.
- Installing refreshes Catalog and Library, returns to Installed, and preserves
  existing responsive/focus/error behavior.
- The conversation Skill picker remains the authority for durable per-chat
  selection; no installation silently selects a Skill.

## Non-goals

- A general-purpose marketplace aggregation protocol.
- skills.sh scraping or dependency.
- Plugin bundles, MCP installation, ratings, reviews, payments, publisher
  accounts, or arbitrary package execution.
- Replacing Neo Chat's current package validator, object store, Library,
  selection, or Agent runtime.
- Installing a whole GitHub repository when no exact Skill path is given.

## Cross-layer contract

```text
GitHub Contents API
  -> bounded catalog DTO
  -> strict frontend schema
  -> Store list/detail

catalog/detail or supported chat link
  -> canonical GitHub coordinate
  -> ref -> exact commit
  -> codeload archive
  -> selected Skill directory
  -> ValidateArchive + canonical ZIP + SBOM
  -> owner-private candidate/install
  -> existing Library
  -> conversation selection / agent_auto runtime
```

Validation is owned by the backend entry/source layers. The frontend treats all
JSON as untrusted and parses it with the existing strict API schema boundary.
PostgreSQL and object storage remain the only installation authorities.

## Acceptance criteria

- Skill Store lists the default OpenAI curated catalog and no longer requires
  admitted Store candidates for that view.
- Selecting a catalog item shows validated metadata and a canonical GitHub
  source link.
- Installing a catalog item produces an ordinary current-user Library entry;
  a second exact install is idempotent.
- A supported GitHub tree/blob Skill link with explicit install intent installs
  deterministically with zero Provider calls.
- Unsupported/malformed/ambiguous GitHub links perform no fetch or mutation.
- A mutable branch/ref is resolved and pinned before package ingestion.
- Source drift between displayed detail and install is rejected.
- GitHub unavailability leaves Installed and conversation selection operational.
- Existing per-conversation Skill selection and bounded `agent_auto` behavior
  remain green.
- Backend focused tests, frontend API/UI tests, Go vet/test, frontend format/
  lint/typecheck/test/build, and the standalone cross-layer gate pass.

## Risks and mitigations

- GitHub unauthenticated API rate limits: use bounded requests, short server
  caching, conditional typed failure, and never make Library depend on catalog
  availability.
- Mutable refs: resolve once to an exact commit and bind install to the
  validated fingerprint.
- Malicious repositories/archives: retain size/file/symlink/path/frontmatter
  validation and never execute source during install.
- Name collisions: source identity is authoritative for install; UI may show
  installed state by the exact source projection when available and must not
  silently replace a different package with the same name.
- Contract drift: add focused source, handler, resource-orchestration, strict
  DTO, server adapter, and Skill Store composition tests.

# Fix LobeHub Skill Installation Fallback

## Goal

Make LobeHub Marketplace Skills installable when the configured Marketplace
M2M token can discover Skills but is rejected by LobeHub's package download
endpoint. Use the exact GitHub Skill path already declared by the LobeHub
detail response and preserve Neo Chat's immutable, validated supply chain.

## Requirements

- Read `manifest.sourceUrl` from the exact LobeHub Skill detail response as an
  internal installation coordinate; never expose it as package authority to
  the browser.
- Accept only the existing strict GitHub `tree`/`blob` Skill URL grammar and
  require its terminal Skill directory name to equal `manifest.name`.
- Resolve mutable GitHub refs to a full commit, enumerate only the declared
  Skill directory through bounded GitHub API responses, fetch only its regular
  blobs, require the selected root `SKILL.md`, and reuse `ValidateArchive`.
- Rebind the validated GitHub bytes to the exact LobeHub
  `lobehub:<identifier>@<version>` source identity so installed-state lookup,
  idempotency, and owner-private Library behavior remain unchanged.
- Stop using LobeHub's `/download` route for Marketplace Skill installation;
  it currently rejects the same valid M2M token that succeeds for discovery.
- Keep LobeHub list/category/detail behavior unchanged.
- Missing, malformed, ambiguous, or manifest-mismatched `sourceUrl` values
  must fail before GitHub network I/O or persistence.
- GitHub/network/archive failures must create no Candidate, object, or Library
  mutation and must remain typed bounded failures.

## Acceptance Criteria

- [x] `openclaw-openclaw-github@1.0.2`-shaped detail installs from its exact
  `skills/github` GitHub path without calling LobeHub `/download`.
- [x] The installed item is found by the original LobeHub identifier/version
  and appears installed on the next detail load.
- [x] A Skill with zero LobeHub `resources` can still install when its exact
  GitHub source validates without downloading the enclosing repository.
- [x] Source path/manifest mismatch is rejected before GitHub requests.
- [x] GitHub source is pinned to a 40-character commit before package fetch.
- [x] Existing archive/path/symlink/owner/fingerprint protections remain in
  force.
- [x] Focused backend regression tests, vet, deployment build, and health check
  pass.

## Definition of Done

- Backend tests prove success, source identity, zero legacy download calls,
  invalid-source rejection, and no mutation on failure.
- Skill supply-chain specs record the Marketplace GitHub-source contract.
- Backend container is rebuilt and healthy.
- Work is committed, archived, and journaled; no push.

## Technical Approach

Extend the sanitized LobeHub detail parser with an internal-only package source
URL. `InstallMarketplaceSkill` passes that exact URL and manifest name through
a GitHub-only subtree transport owned by `DirectSkillLinkSource`. The transport
reuses the strict URL parser and ref pinning, but enumerates only the exact
directory at the pinned commit, rejects symlinks/submodules and bounded-limit
violations, fetches verified Git blobs, and builds one canonical ZIP for the
existing validator. Before ingestion, only the source identity fields are
rewritten to the exact LobeHub identifier/version.

## Decision (ADR-lite)

**Context**: LobeHub accepts the configured M2M access token for list and
category APIs but returns `401 invalid_token` from `/skills/{id}/download` for
both zero-resource and populated-resource Skills.

**Decision**: Use the exact `manifest.sourceUrl` GitHub coordinate as the
package transport while retaining LobeHub as the product/source identity.

**Consequences**: Installation no longer depends on an unavailable LobeHub
download permission. Mutable GitHub refs are safely pinned, but a future
source repository deletion or source/path mismatch correctly makes the Skill
uninstallable rather than weakening validation.

## Out of Scope

- Requesting or rotating LobeHub OAuth scopes/credentials.
- Accepting repository roots, arbitrary webpages, install commands, or
  non-GitHub package sources.
- Executing any upstream Skill script, hook, npm package, or installer.
- Changing the Skill Store layout, taxonomy, or per-Skill artwork.
- Full repository/standalone test suites for this focused backend repair.

## Technical Notes

- Backend owners:
  `internal/skillsupply/marketplace.go`, `sources.go`, and focused tests.
- The existing direct GitHub pipeline owns URL parsing and ref pinning. The
  Marketplace path adds an exact-directory transport because codeload returns
  the entire repository and cannot install a small Skill from a repository
  larger than the package archive limit.
- See [`research/lobehub-download-auth.md`](research/lobehub-download-auth.md).

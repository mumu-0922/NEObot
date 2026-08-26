# Direct Skill source research

## External source facts

- AIHero's `/skills-grill-me` page publishes the command
  `npx skills@latest add mattpocock/skills --skill=grill-me` and links the
  `mattpocock/skills` GitHub repository:
  <https://www.aihero.dev/skills-grill-me>.
- The repository's current file is under
  `skills/productivity/grill-me/SKILL.md`; the page and repository therefore
  expose enough data to derive `{owner, repository, skill slug}` without
  executing the published command:
  <https://github.com/mattpocock/skills/blob/main/skills/productivity/grill-me/SKILL.md>.
- skills.sh documents the same source/slug identity shape and exposes complete
  Skill file snapshots through its detail API, but that API currently returns
  HTTP 401 without Vercel OIDC in this deployment. It is not a reliable Neo
  Chat runtime dependency:
  <https://www.skills.sh/docs/api>.
- skills.sh explicitly warns that listed Skills are not guaranteed safe. The
  product requirement intentionally lets the owner trust/install anyway; Neo
  Chat still must preserve structural and ownership integrity.

## Existing Neo Chat seams

- `skillsupply.GitHubSource` already performs strict GitHub URL parsing,
  bounded codeload retrieval, exact 40-character commit pinning, redirect
  constraints, and subdirectory stripping.
- `skillsupply.ValidateArchive` already rejects traversal, symlinks, duplicate
  names, oversized/over-expanded files, nested `SKILL.md`, invalid frontmatter,
  and malformed runtime manifests. It produces a deterministic canonical ZIP,
  package fingerprint, runtime fingerprint, and SBOM.
- `skillsupply.Service.ingest` already writes quarantine/package/SBOM objects
  before an immutable candidate transaction.
- The blocking assumptions are `skill_package_candidates.status='admitted'`,
  the installation trigger, `GetStoreItem`, and runtime hydration through
  Store-only reads.

## Chosen implementation direction

Introduce an owner-private candidate trust mode rather than faking an admin
review or writing loose files into the runtime cache:

1. Resolve one supported discovery page to a constrained repository/slug.
2. Resolve GitHub HEAD to an exact commit and fetch the exact commit ZIP.
3. Discover exactly one Skill directory whose validated frontmatter name
   matches the slug.
4. Persist canonical immutable package bytes plus an owner-private direct
   candidate.
5. Install it only for that owner, reusing Library/uninstall/runtime behavior.

This bypasses Store review exactly as requested while retaining deterministic
bytes, idempotency, cleanup, and cross-user isolation. Running `npx` or copying
directly into `data/agent-skills` would bypass the product's Library and runtime
authority and is therefore the wrong seam.

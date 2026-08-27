# Codex Skill installation research

## Official product behavior

Primary source: OpenAI's Codex Skills documentation at
`https://learn.chatgpt.com/docs/build-skills` and the installed Codex system
Skill installer in `/home/mumu/.codex/skills/.system/skill-installer/`.

- A Skill is a directory with a required `SKILL.md`; `scripts/`, `references/`,
  `assets/`, and `agents/openai.yaml` are optional.
- Codex initially indexes only Skill name, description, and location. It loads
  the full `SKILL.md` only after explicit `$skill`/`/skills` selection or
  implicit description matching.
- The built-in installer lists the curated catalog from
  `openai/skills:skills/.curated` by using the GitHub Contents API.
- It accepts GitHub repository/path input, downloads a public repository archive
  first and falls back to Git sparse checkout, requires `SKILL.md`, and installs
  the selected directory under the local Skills root.
- The experimental catalog is separate (`skills/.experimental`); it is not part
  of the default curated experience.
- Plugins are an optional distribution bundle for skills, MCP servers, and apps;
  they are not required for a standalone Skill catalog MVP.

## Neo Chat mapping

Codex's local copy operation is not a sufficient multi-user server trust model.
Neo Chat already has the stronger reusable pieces:

- exact Git commit/codeload fetching in `internal/skillsupply`;
- path, archive-size, file-count, symlink, frontmatter, and runtime-manifest
  validation;
- canonical ZIP and SBOM generation;
- content-addressed object storage and owner-private installation;
- current-user Library and revision-bound uninstall;
- per-conversation Skill selection;
- runtime materialization and bounded lexical `agent_auto` activation.

Therefore the correct adaptation is not to port the Python installer or invoke
its shell commands. It is to add the same curated discovery/source-coordinate
model in front of Neo Chat's existing package authority.

## Chosen decisions

1. Default catalog authority: fixed `openai/skills`, path `skills/.curated`, ref
   `main`.
2. Mutable refs are display/discovery inputs only. Every detail/install resolves
   to an exact commit before archive ingestion.
3. GitHub tree/blob links are supported direct-install inputs; repository-root
   links remain rejected because they do not identify one Skill directory.
4. AIHero direct links remain compatible, but they are no longer the public
   catalog model.
5. Existing admitted Store endpoints may remain as internal/backward-compatible
   APIs while the user-facing Skill Store consumes the new curated catalog.
6. Installation never changes conversation selection. The existing picker and
   `agent_auto` runtime retain that authority.
7. The UI hides internal review/admission machinery and presents source,
   description, compatibility, and install state.

## Required test seams

- GitHub Contents listing bounds and strict response parsing.
- ref-to-commit resolution, exact codeload URL, and selected subdirectory.
- tree/blob URL parsing, traversal/query/fragment/multiple-link denial.
- archive/fingerprint drift rejection and duplicate install reconciliation.
- deterministic explicit chat install with zero Provider calls.
- frontend strict DTO parsing, list/detail/install error states, Installed/Store
  refresh, responsive drill-in, and internal-control-language absence.

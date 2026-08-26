# AIHero Skill link boundary

## Source inspected

- https://www.aihero.dev/skills-grill-me (read-only, 2026-08-26)

## Findings

- The page identifies the Skill as `grill-me` and describes it as a user-invoked, stateless
  interview Skill.
- The page displays `npx skills@latest add mattpocock/skills --skill=grill-me` and links to
  `mattpocock/skills` as its upstream source.
- The page URL and displayed command do not pin an immutable commit, package fingerprint, or
  admitted Neo Chat candidate revision.

## Neo Chat mapping

- Treat AIHero as a discovery alias, never as installation authority.
- Accept only HTTPS URLs on `www.aihero.dev` or `aihero.dev` with an exact
  `/skills-<lowercase-slug>` path and no userinfo, query, fragment, custom port, or extra segment.
- Convert the path to the sanitized Skill identifier `<lowercase-slug>` and pass it through the
  existing admitted Skill search/install flow.
- Never execute or reproduce the page's `npx` command. Installation still requires an exact
  admitted package fingerprint and all existing owner/admin/supply-chain checks.


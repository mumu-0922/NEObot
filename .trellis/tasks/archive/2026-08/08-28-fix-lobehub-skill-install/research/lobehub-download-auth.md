# LobeHub download authentication diagnosis

Sanitized live diagnosis on 2026-08-28. No access token, client secret, package
content, or user data was persisted.

## Observed boundary

The same freshly issued Marketplace client-credentials token returned:

- `200 application/json` for `/api/v1/skills`;
- `200 application/json` for `/api/v1/skills/categories`;
- `401 application/json` with `invalid_token` for
  `/api/v1/skills/openclaw-openclaw-github/download?version=1.0.2`;
- the same `401 invalid_token` for
  `/api/v1/skills/anthropics-skills-pdf/download?version=1.0.4`.

This proves the failure is a transport/authorization mismatch, not caused by
the selected Skill's zero-resource projection. The live application request
returned `502` after the backend mapped that upstream download rejection to
`SKILL_SOURCE_UNAVAILABLE`.

## Available exact source

Public exact detail responses contain `manifest.sourceUrl`, for example:

- `https://github.com/openclaw/openclaw/tree/main/skills/github`
- `https://github.com/anthropics/skills/tree/main/skills/pdf`

These URLs already fit Neo Chat's strict direct GitHub Skill parser. The parser
can still resolve the mutable ref to a commit and bind one exact directory.

The generic codeload path is not a viable Marketplace fallback. A sanitized
live probe of `openclaw/openclaw` at its resolved commit received more than
96,949,899 bytes before the 45-second request timed out. Neo Chat's source
archive ceiling is 32 MiB, while the requested `skills/github` directory
contains only one 3,048-byte `SKILL.md`. Downloading the enclosing repository
would therefore fail the exact user case and waste bandwidth.

## Chosen repair

Treat `manifest.sourceUrl` as internal package transport, not browser or install
authority. Validate it with the existing parser, require the manifest-name
match, pin its ref, enumerate only the exact directory through bounded GitHub
API responses, reject non-regular entries, verify each Git blob identity, and
build a canonical ZIP for `ValidateArchive`. Rebind only the source identity to
`lobehub:<identifier>@<version>` before canonical ingestion.

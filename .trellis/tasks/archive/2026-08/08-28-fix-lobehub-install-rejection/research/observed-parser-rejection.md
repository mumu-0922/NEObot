# Observed parser rejection

Sanitized diagnosis on 2026-08-28; no cookies, credentials, user content, or
raw application response bodies were persisted.

## Request evidence

Backend logs recorded Marketplace install POST attempts at 12:37:57,
12:38:07, and 12:38:17 UTC. Each returned HTTP `400`, response bytes `80`, and
completed after the detail/source path was entered. Neo Chat's serialized
`INVALID_SKILL_PACKAGE` response is 79 bytes plus the JSON newline written by
the handler, matching the observation exactly.

## Exact source evidence

The public LobeHub detail for `openclaw-openclaw-summarize` reports:

- exact version `1.0.3`;
- manifest name `summarize`;
- source URL `https://github.com/openclaw/openclaw/tree/main/skills/summarize`.

GitHub resolves the source to a full commit and returns one regular root file:
`skills/summarize/SKILL.md`, 2,125 bytes. The document has valid root
frontmatter, but its `metadata` value contains a nested `openclaw` mapping,
emoji, requirements, and an install array.

## Parser boundary

`decodeFrontmatter` calls `scalarMap` for `metadata`. `scalarMap` rejects every
non-string value, so an otherwise valid community Skill becomes
`ErrManifestInvalid`, which the HTTP handler maps to `INVALID_SKILL_PACKAGE`.

The nested values are platform hints and must remain inert. Compatibility
requires accepting bounded nested values without copying them into Neo Chat's
string metadata authority or executing any declared installer.

## Verification

- OpenClaw-shaped parser and archive regression: passed.
- Marketplace exact-detail → pinned GitHub subtree → owner Library regression:
  passed, including the next detail's installed state.
- `go test -race ./internal/skillsupply`, `go vet ./internal/skillsupply`,
  `go test ./...`, and `go vet ./...`: passed.
- Security scanner: 26 files scanned, zero critical/high/medium/low findings.
- Backend image rebuilt; container health and `/ready` database/Redis/storage
  checks are all ready.

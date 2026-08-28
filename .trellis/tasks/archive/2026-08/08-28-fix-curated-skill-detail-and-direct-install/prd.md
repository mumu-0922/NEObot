# Fix curated Skill detail and direct installation

## Problem

The fixed `openai/skills` curated catalog lists the official `pdf` Skill, but
opening its detail fails frontend runtime validation. An explicit direct-install
request using a non-existent neighboring path such as `.../.curated/pdfs`
downloads the repository archive and is then reported as a generic package
validation failure, which hides that the requested Skill directory does not
exist.

The upstream `pdf/SKILL.md` intentionally omits both `metadata.version` and
`allowed-tools`. Neo Chat already derives a deterministic synthetic version for
versionless Skills, but its catalog detail projection serializes the empty Tool
list as JSON `null`. The strict frontend contract correctly accepts only a JSON
array.

## Goals

1. Catalog detail and installation must support official instruction-only
   Skills whose `allowed-tools` field is absent.
2. Every Skill DTO collection owned by this path must serialize an empty Tool
   list as `[]`, never `null`.
3. An exact GitHub directory install must prove that the selected directory
   contains its root `SKILL.md` in the pinned archive before package ingestion.
4. A missing or invalid exact directory must fail without mutation and produce
   truthful bounded user copy indicating that the link is invalid or the Skill
   directory does not exist.
5. Keep existing immutable commit resolution, package validation, owner-private
   installation, zero-execution, and strict frontend response validation.

## Non-goals

- Do not auto-correct, fuzzy-match, or guess `pdfs` as `pdf`.
- Do not weaken the frontend schema to accept `null` collections.
- Do not invent a semantic version; retain the existing deterministic
  content-fingerprint fallback.
- Do not restore marketplace review or execute repository commands.

## Acceptance criteria

- `GET /v1/skills/catalog/items/pdf` returns a strict detail DTO with
  `allowedTools: []` and a non-empty deterministic version.
- The exact URL
  `https://github.com/openai/skills/tree/main/skills/.curated/pdf` can complete
  the existing owner-private direct-install flow.
- The non-existent URL ending in `skills/.curated/pdfs` is rejected before
  ingestion/storage and the deterministic Chat answer says the link is invalid
  or its Skill directory does not exist.
- Focused Go tests cover missing `allowed-tools`, exact selected-directory
  presence, and safe user-facing failure copy.
- Existing frontend strict Zod parsing remains unchanged and focused API tests
  stay green.

## Verification

- `gofmt` on changed Go files.
- Focused `go test` for `internal/skillsupply` and `internal/chat`.
- `go vet ./...` and `go test ./...` from `mm-chat/backend`.
- Focused frontend Skill Store API test to prove the unchanged boundary.
- Rebuild/restart the affected service and verify Compose health.


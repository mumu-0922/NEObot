# OpenAI curated PDF Skill contract research

## Upstream evidence

- Repository: `openai/skills`
- Fixed Neo Chat catalog path: `skills/.curated`
- Current `main` commit observed on 2026-08-28:
  `49f948faa9258a0c61caceaf225e179651397431`
- Correct Skill directory: `skills/.curated/pdf`
- Incorrect directory from the failed test: `skills/.curated/pdfs`
- Official raw file:
  `https://raw.githubusercontent.com/openai/skills/main/skills/.curated/pdf/SKILL.md`

The official frontmatter contains only:

```yaml
name: "pdf"
description: "Use when tasks involve reading, creating, or reviewing PDF files ..."
```

It has neither `metadata.version` nor `allowed-tools`. This is a valid Codex
instruction-only Skill and must remain valid in Neo Chat.

## Local data-flow finding

```text
GitHub codeload ZIP
  -> ValidateArchive
  -> PackageVersion.AllowedTools
  -> CatalogSkill.AllowedTools
  -> encoding/json
  -> frontend strict Zod array
```

`parseAllowedTools("")` returns an allocated empty slice, but the later
`append([]string(nil), values...)` copy collapses it back to `nil`. Go JSON then
emits `null`, while the frontend contract requires `[]`.

The package already has a shared `nonNilStrings` helper at the PostgreSQL DTO
boundary. Reuse it at validation and catalog projection boundaries instead of
weakening the frontend parser.

## Missing-directory finding

GitHub codeload serves the complete repository archive even when the requested
subdirectory is wrong, because the subdirectory is a local extraction prefix,
not part of the codeload request. `GitHubSource.Fetch` therefore must verify
that `<archive-root>/<selected-subdirectory>/SKILL.md` exists whenever a
subdirectory was explicitly selected. Absence is an invalid exact source, not
a malformed Skill package.

## Relevant executable specs

- `.trellis/spec/backend/skill-supply-chain.md`
- `.trellis/spec/backend/resource-orchestration.md`
- `.trellis/spec/frontend/skill-store.md`
- `.trellis/spec/frontend/type-safety.md`
- `.trellis/spec/frontend/quality-guidelines.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`

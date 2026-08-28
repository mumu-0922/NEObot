# Hide internal Skill versions from UI

## Goal

Remove Skill version and content-fingerprint labels from all user-facing
frontend surfaces so internal fallback values such as
`v0.0.0+63fe34c83e72` do not clutter the product UI.

## What I already know

- The installed Skill card currently renders `v{entry.version}`.
- Skill versions are also visible in the Conversation picker, LobeHub cards,
  LobeHub detail, and OpenAI curated detail.
- Backend/API version fields remain required for exact installation, update,
  identity, and audit behavior.

## Requirements

- Hide Skill versions from installed cards and the Conversation Skill picker.
- Hide Skill versions from marketplace cards and both Skill detail variants.
- Keep version data and exact-version install behavior unchanged outside the
  presentation layer.
- Keep names, descriptions, source, author, license, install count, rating,
  compatibility, and permissions visible as before.
- Add focused regression coverage for the presentation contract.

## Acceptance Criteria

- [x] No user-facing Skill card, picker item, or detail dialog renders a version.
- [x] Internal fallback fingerprints such as `0.0.0+<hash>` remain available to
      the install/runtime contract but are not displayed.
- [x] Focused tests, formatting, lint, and typecheck pass.
- [x] The active Frontend is rebuilt and healthy.

## Definition of Done

- Frontend source and Skill UI spec agree.
- A focused conventional commit is created.
- The task is archived and recorded in the Trellis journal.

## Out of Scope

- Changing Backend version generation or package identity.
- Changing MCP Server versions or unrelated application version displays.
- Redesigning the Skill cards or detail dialog.

## Technical Notes

- Primary files: `SkillStorePrimitives.tsx`,
  `SkillMarketplacePrimitives.tsx`, `SkillDetailDialog.tsx`, and
  `ConversationResourcePickers.tsx`.
- This is a presentation-only change; the DTO and API payloads retain version.
